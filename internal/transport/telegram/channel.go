package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	coreCache "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/cache"
	coreErrors "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/errors"
	coreMatcher "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/matcher"
	coreProfile "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/profile"
	coreResolver "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/resolver"
	coreRuntime "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/runtime"
	coreSync "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/syncclient"
	coreTransform "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/transform"
)

type Channel struct {
	channelID string
	botToken  string
	baseURL   string
	http      *http.Client
	pollTO    int
	matcher   *coreMatcher.Matcher
	runtime   *coreRuntime.Runtime
	profiles  []coreProfile.Profile
	affinity  *coreCache.Affinity
	offset    int64
}

type envelope struct{ data map[string]string }

func (e *envelope) SourceKey() string { return "telegram" }
func (e *envelope) GetField(location, key string) (string, bool) {
	v, ok := e.data[location+"."+key]
	return v, ok
}
func (e *envelope) SetField(location, key, value string) { e.data[location+"."+key] = value }

type parsedMapRef struct {
	Ref   string
	Steps []coreTransform.Spec
}

func parseMapRef(raw string) parsedMapRef {
	parts := strings.Split(raw, "|")
	out := parsedMapRef{Ref: strings.TrimSpace(parts[0])}
	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		if p != "" {
			out.Steps = append(out.Steps, coreTransform.Spec{Type: p})
		}
	}
	return out
}

func New(channelID, botToken, c2SyncBaseURL string, pollTimeout int, profiles []coreProfile.Profile) *Channel {
	return &Channel{
		channelID: channelID,
		botToken:  botToken,
		baseURL:   "https://api.telegram.org/bot" + botToken,
		http:      &http.Client{Timeout: 35 * time.Second},
		pollTO:    pollTimeout,
		matcher:   coreMatcher.New(),
		runtime:   coreRuntime.New(coreSync.NewHTTPClient(c2SyncBaseURL, nil)),
		profiles:  profiles,
		affinity:  coreCache.NewAffinity(30 * time.Minute),
	}
}

func (c *Channel) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		updates, err := c.getUpdates(ctx)
		if err != nil {
			return err
		}
		for _, u := range updates {
			if u.UpdateID >= c.offset {
				c.offset = u.UpdateID + 1
			}
			if u.Message == nil || u.Message.Text == "" {
				continue
			}
			parsed, ok := ParseText(u.Message.Text)
			if !ok {
				continue
			}
			if err := c.handleMessage(ctx, u.Message.Chat.ID, parsed); err != nil {
				_ = c.sendMessage(ctx, u.Message.Chat.ID, "error: "+err.Error())
			}
		}
	}
}

func (c *Channel) handleMessage(ctx context.Context, chatID int64, in ParsedInbound) error {
	source := strconv.FormatInt(chatID, 10)
	hint := in.ProfileID
	if hint == "" {
		if p, ok := c.affinity.Get(source); ok {
			hint = p
		}
	}
	res, err := c.matcher.Resolve(ctx, hint, c.profiles)
	if err != nil {
		return err
	}

	input := coreResolver.Input{Body: map[string]any{"profile_id": in.ProfileID, "id": in.ID, "encrypted_data": in.EncryptedData}}
	idMap := parseMapRef(res.Profile.Mapping.ID)
	encMap := parseMapRef(res.Profile.Mapping.EncryptedData)
	idRaw, ok, err := coreResolver.Resolve(idMap.Ref, input)
	if err != nil || !ok {
		return fmt.Errorf("id not found by profile mapping")
	}
	encRaw, ok, err := coreResolver.Resolve(encMap.Ref, input)
	if err != nil || !ok {
		return fmt.Errorf("encrypted_data not found by profile mapping")
	}
	id, err := coreTransform.ApplyDecode(idRaw, idMap.Steps)
	if err != nil {
		return err
	}
	enc, err := coreTransform.ApplyDecode(encRaw, encMap.Steps)
	if err != nil {
		return err
	}

	p := res.Profile
	p.Mapping.ID = idMap.Ref
	p.Mapping.EncryptedData = encMap.Ref
	if p.Mapping.ProfileID != "" {
		p.Mapping.ProfileID = parseMapRef(p.Mapping.ProfileID).Ref
	}
	env := &envelope{data: map[string]string{}}
	env.SetField("mapping", p.Mapping.ID, id)
	env.SetField("mapping", p.Mapping.EncryptedData, enc)
	if hint != "" && p.Mapping.ProfileID != "" {
		env.SetField("mapping", p.Mapping.ProfileID, hint)
	}

	out, err := c.runtime.HandleWithProfile(ctx, env, c.channelID, p)
	if err != nil {
		if code := coreErrors.Code(err); code != "" {
			return fmt.Errorf("%s: %w", code, err)
		}
		return err
	}

	outID, _ := coreTransform.ApplyEncode(out.ID, idMap.Steps)
	outEnc, _ := coreTransform.ApplyEncode(out.EncryptedData, encMap.Steps)
	c.affinity.Set(source, p.ProfileID)
	msg := "id:" + outID + "\n" + outEnc
	return c.sendMessage(ctx, chatID, msg)
}

func (c *Channel) getUpdates(ctx context.Context) ([]Update, error) {
	u := c.baseURL + "/getUpdates?timeout=" + strconv.Itoa(c.pollTO) + "&offset=" + strconv.FormatInt(c.offset, 10)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out getUpdatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Result, nil
}

func (c *Channel) sendMessage(ctx context.Context, chatID int64, text string) error {
	body, _ := json.Marshal(map[string]any{"chat_id": chatID, "text": text})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/sendMessage", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram sendMessage status %d", resp.StatusCode)
	}
	return nil
}
