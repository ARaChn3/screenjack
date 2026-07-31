package main

import (
	"os"
	"path/filepath"

	"encoding/json"
)

// Config holds user preferences
type Config struct {
	ServerPort  int    `json:"server_port"`
	DefaultOS   string `json:"default_os"`   // "linux" or "windows"
	EmbedAsset  bool   `json:"embed_asset"`  // default embed mode
	PayloadName string `json:"payload_name"` // default payload filename
	AssetDir    string `json:"asset_dir"`    // custom asset directory
	LastAsset   string `json:"last_asset"`   // remember last selected asset
}

var defaultConfig = Config{
	ServerPort:  8000,
	DefaultOS:   "linux",
	EmbedAsset:  true,
	PayloadName: "screenjack",
	AssetDir:    "../assets",
}

func configPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "screenjack", "config.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "screenjack", "config.json")
}

func LoadConfig() Config {
	cfg := defaultConfig
	data, err := os.ReadFile(configPath())
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func SaveConfig(cfg Config) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
