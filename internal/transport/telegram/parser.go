package telegram

import "strings"

type ParsedInbound struct {
	ProfileID     string
	ID            string
	EncryptedData string
}

// ParseText expects one of:
// 1) p:<profile>\nid:<id>\n<encrypted_data>
// 2) id:<id>\n<encrypted_data>
func ParseText(text string) (ParsedInbound, bool) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) < 2 {
		return ParsedInbound{}, false
	}
	idx := 0
	out := ParsedInbound{}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(lines[0])), "p:") {
		out.ProfileID = strings.TrimSpace(lines[0][2:])
		idx = 1
	}
	if idx >= len(lines) || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(lines[idx])), "id:") {
		return ParsedInbound{}, false
	}
	out.ID = strings.TrimSpace(lines[idx][3:])
	if idx+1 >= len(lines) {
		return ParsedInbound{}, false
	}
	out.EncryptedData = strings.TrimSpace(strings.Join(lines[idx+1:], "\n"))
	if out.ID == "" || out.EncryptedData == "" {
		return ParsedInbound{}, false
	}
	return out, true
}
