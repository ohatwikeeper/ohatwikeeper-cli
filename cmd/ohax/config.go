package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type config struct {
	UUID   string `json:"uuid"`
	APIKey string `json:"api_key,omitempty"`
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ohax", "config.json"), nil
}

func loadConfig() config {
	var c config
	p, err := configPath()
	if err != nil {
		return c
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return c
	}
	_ = json.Unmarshal(b, &c)
	return c
}

func saveConfig(c config) (string, error) {
	p, err := configPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return p, os.WriteFile(p, append(b, '\n'), 0o644)
}

// defaultUUID: OHAX_UUID → 保存済み設定
func defaultUUID() string {
	if v := os.Getenv("OHAX_UUID"); v != "" {
		if u, _, err := parseTarget(v); err == nil {
			return u
		}
	}
	return loadConfig().UUID
}

func cmdUse() int {
	fmt.Fprintln(os.Stderr, paint(cRed, "✗")+" ohax use は廃止されました。ログインしたアカウントが既定ユーザーになります。")
	fmt.Fprintln(os.Stderr, dim("  ")+paint(cSky, "ohax login")+dim(" でログインしてください(Lapount)"))
	return 2
}
