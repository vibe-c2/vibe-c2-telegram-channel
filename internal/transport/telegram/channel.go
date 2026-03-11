package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	coreErrors "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/errors"
	coreMatcher "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/matcher"
	coreProfile "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/profile"
	coreRuntime "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/runtime"
	coreSync "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/syncclient"
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
	offset    int64
}

type envelope struct{ data map[string]string }

func (e *envelope) SourceKey() string { return "telegram" }
func (e *envelope) GetField(location, key string) (string, bool) {
	v, ok := e.data[location+"."+key]
	return v, ok
}
func (e *envelope) SetField(location, key, value string) { e.data[location+"."+key] = value }

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
	res, err := c.matcher.Resolve(ctx, in.ProfileID, c.profiles)
	if err != nil {
		return err
	}
	env := &envelope{data: map[string]string{}}
	env.SetField("mapping", res.Profile.Mapping.ID, in.ID)
	env.SetField("mapping", res.Profile.Mapping.EncryptedData, in.EncryptedData)
	if in.ProfileID != "" && res.Profile.Mapping.ProfileID != "" {
		env.SetField("mapping", res.Profile.Mapping.ProfileID, in.ProfileID)
	}

	out, err := c.runtime.HandleWithProfile(ctx, env, c.channelID, res.Profile)
	if err != nil {
		if code := coreErrors.Code(err); code != "" {
			return fmt.Errorf("%s: %w", code, err)
		}
		return err
	}

	msg := "id:" + out.ID + "\n" + out.EncryptedData
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
