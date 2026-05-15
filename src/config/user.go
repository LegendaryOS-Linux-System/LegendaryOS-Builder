package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UserConfig represents user.toml — personal identity settings.
// Kept separate from config.toml so it can be gitignored.
// Contains GitHub credentials, organization/user info.
type UserConfig struct {
	GitHub GitHubConfig `toml:"github"`
}

type GitHubConfig struct {
	// Type: "user" or "org"
	Type string `toml:"type"`
	// Name: GitHub username or organization name
	Name string `toml:"name"`
	// Token: optional stored token (can also be passed interactively)
	// Leave empty to always prompt — recommended for security.
	Token string `toml:"token"`
}

// LoadUser reads user.toml from the project root.
// If the file doesn't exist, returns a zeroed UserConfig (not an error).
func LoadUser(root string) (*UserConfig, error) {
	path := filepath.Join(root, "user.toml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &UserConfig{}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open user.toml: %w", err)
	}
	defer f.Close()

	uc := &UserConfig{}
	section := ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line, "[]")
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		// strip inline comment
		if ci := strings.Index(val, " #"); ci >= 0 {
			val = strings.TrimSpace(val[:ci])
		}
		val = strings.Trim(val, `"'`)

		if section == "github" {
			switch key {
			case "type":
				uc.GitHub.Type = val
			case "name":
				uc.GitHub.Name = val
			case "token":
				uc.GitHub.Token = val
			}
		}
	}
	return uc, scanner.Err()
}

// Registry returns the full ghcr.io registry prefix for this user/org.
// e.g. "ghcr.io/LegendaryOS-Linux-System"
func (uc *UserConfig) Registry() string {
	if uc.GitHub.Name == "" {
		return ""
	}
	return "ghcr.io/" + uc.GitHub.Name
}

// GitHubURL returns the GitHub profile/org URL.
func (uc *UserConfig) GitHubURL() string {
	if uc.GitHub.Name == "" {
		return ""
	}
	return "https://github.com/" + uc.GitHub.Name
}
