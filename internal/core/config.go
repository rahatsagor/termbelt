package core

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	IPAPIURL       string   `json:"ip_api_url,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	Favorites      []string `json:"favorites"`
}

func ConfigPath() string {
	if p := os.Getenv("TERMBELT_CONFIG"); p != "" {
		return p
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "termbelt", "config.json")
}
func loadConfigFile() (Config, error) {
	c := Config{TimeoutSeconds: 20, Favorites: []string{"speed", "ip", "domains", "site", "json", "uuid"}}
	if p := ConfigPath(); p != "" {
		f, err := os.Open(p)
		if err == nil {
			b, readErr := readLimited(f, 1<<20)
			f.Close()
			if readErr != nil {
				return c, fmt.Errorf("cannot read config %s: %w", p, readErr)
			}
			if !strings.HasPrefix(strings.TrimSpace(string(b)), "{") {
				return c, fmt.Errorf("invalid config %s: expected a JSON object", p)
			}
			if err = json.Unmarshal(b, &c); err != nil {
				return c, fmt.Errorf("invalid config %s: %w", p, err)
			}
		} else if !os.IsNotExist(err) {
			return c, err
		}
	}
	return normalizeConfig(c)
}

func LoadConfig() (Config, error) {
	c, err := loadConfigFile()
	if err != nil {
		return c, err
	}
	if v := os.Getenv("TERMBELT_IP_API_URL"); v != "" {
		c.IPAPIURL = v
	}
	return normalizeConfig(c)
}

func normalizeConfig(c Config) (Config, error) {
	if c.TimeoutSeconds < 3 || c.TimeoutSeconds > 120 {
		return c, fmt.Errorf("timeout_seconds must be 3–120")
	}
	if c.IPAPIURL != "" {
		if _, err := ValidateIPAPI(c.IPAPIURL); err != nil {
			return c, err
		}
	}
	seen := map[string]bool{}
	favorites := make([]string, 0, len(Tools))
	for _, id := range c.Favorites {
		if _, ok := FindTool(id); ok && !seen[id] {
			favorites = append(favorites, id)
			seen[id] = true
		}
	}
	c.Favorites = favorites
	return c, nil
}
func SaveConfig(c Config) error {
	c, err := normalizeConfig(c)
	if err != nil {
		return err
	}
	p := ConfigPath()
	if p == "" {
		return fmt.Errorf("cannot find user config directory")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(append(b, '\n'))
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), p)
}

// SaveFavorites preserves settings from disk, including when this run uses
// temporary environment or command-line overrides.
func SaveFavorites(favorites []string) error {
	c, err := loadConfigFile()
	if err != nil {
		return err
	}
	c.Favorites = favorites
	return SaveConfig(c)
}
func (c Config) IsFavorite(id string) bool {
	for _, v := range c.Favorites {
		if v == id {
			return true
		}
	}
	return false
}
func (c *Config) ToggleFavorite(id string) {
	for i, v := range c.Favorites {
		if v == id {
			c.Favorites = append(c.Favorites[:i], c.Favorites[i+1:]...)
			return
		}
	}
	c.Favorites = append(c.Favorites, id)
}
func (c *Config) Set(key, value string) error {
	stored, err := loadConfigFile()
	if err != nil {
		return err
	}
	switch key {
	case "ip-api-url":
		value = strings.TrimRight(value, "/")
		if value != "" {
			if _, err := ValidateIPAPI(value); err != nil {
				return err
			}
		}
		stored.IPAPIURL = value
	case "timeout-seconds":
		n, err := strconv.Atoi(value)
		if err != nil || n < 3 || n > 120 {
			return fmt.Errorf("timeout-seconds must be 3–120")
		}
		stored.TimeoutSeconds = n
	default:
		return fmt.Errorf("unknown setting %q (supported: ip-api-url, timeout-seconds)", key)
	}
	if err := SaveConfig(stored); err != nil {
		return err
	}
	*c = stored
	return nil
}

func ValidateIPAPI(value string) (*url.URL, error) {
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, fmt.Errorf("IP API must be an absolute http(s) URL without credentials, query or fragment")
	}
	if _, err := parseHTTPURL(value); err != nil {
		return nil, fmt.Errorf("invalid IP API URL: %w", err)
	}
	return u, nil
}
