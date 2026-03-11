package config

import (
	"fmt"
	"os"

	coreProfile "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/profile"
	"gopkg.in/yaml.v3"
)

type Config struct {
	ChannelID     string `yaml:"channel_id"`
	BotToken      string `yaml:"bot_token"`
	C2SyncBaseURL string `yaml:"c2_sync_base_url"`
	ProfilesFile  string `yaml:"profiles_file"`
	PollTimeout   int    `yaml:"poll_timeout_seconds"`
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if c.ChannelID == "" {
		c.ChannelID = "telegram-main"
	}
	if c.PollTimeout <= 0 {
		c.PollTimeout = 30
	}
	if c.ProfilesFile == "" {
		c.ProfilesFile = "configs/profiles.example.yaml"
	}
	if c.BotToken == "" {
		return Config{}, fmt.Errorf("bot_token is required")
	}
	if c.C2SyncBaseURL == "" {
		return Config{}, fmt.Errorf("c2_sync_base_url is required")
	}
	return c, nil
}

func LoadProfiles(path string) ([]coreProfile.Profile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profiles: %w", err)
	}
	profiles, err := coreProfile.ParseYAMLProfiles(b)
	if err != nil {
		return nil, fmt.Errorf("parse profiles: %w", err)
	}
	return profiles, nil
}
