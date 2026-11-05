package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lsongdev/mihome-go/mihome"
	"gopkg.in/yaml.v3"
)

// Config is <data directory>/config.yaml. Fields ending in _env name
// environment variables, which win over the value in the file.
type Config struct {
	Account struct {
		Username    string `yaml:"username"`
		UsernameEnv string `yaml:"username_env"`
		Password    string `yaml:"password"`
		PasswordEnv string `yaml:"password_env"`
		// TokenPath keeps the login state: the device id and the tokens.
		TokenPath string `yaml:"token_path"`
	} `yaml:"account"`
	API struct {
		Listen   string `yaml:"listen"`
		Token    string `yaml:"token"`
		TokenEnv string `yaml:"token_env"`
	} `yaml:"api"`
	Database struct {
		Path string `yaml:"path"`
	} `yaml:"database"`
	Cache struct {
		DevicesTTL time.Duration `yaml:"devices_ttl"`
		SpecTTL    time.Duration `yaml:"spec_ttl"`
	} `yaml:"cache"`
	Spec struct {
		Language    string `yaml:"language"`
		LanguageEnv string `yaml:"language_env"`
	} `yaml:"spec"`
	Events struct {
		// OAuthClientID turns on realtime events.
		OAuthClientID string `yaml:"oauth_client_id"`
		IncludeShared bool   `yaml:"include_shared"`
	} `yaml:"events"`
}

func defaultConfig() Config {
	var c Config
	c.Account.UsernameEnv = "MI_USERNAME"
	c.Account.PasswordEnv = "MI_PASSWORD"
	c.Account.TokenPath = "mi.token"
	c.API.Listen = "127.0.0.1:8080"
	c.API.TokenEnv = "MI_API_TOKEN"
	c.Database.Path = "mihome.db"
	c.Cache.DevicesTTL = time.Minute
	c.Cache.SpecTTL = 30 * 24 * time.Hour
	c.Spec.Language = mihome.DefaultSpecLanguage
	c.Spec.LanguageEnv = "MI_SPEC_LANGUAGE"
	return c
}

func fromEnv(env, value string) string {
	if env != "" {
		if fromEnv := os.Getenv(env); fromEnv != "" {
			return fromEnv
		}
	}
	return value
}

func (c Config) specLanguage() string { return fromEnv(c.Spec.LanguageEnv, c.Spec.Language) }

func (c Config) username() string { return fromEnv(c.Account.UsernameEnv, c.Account.Username) }
func (c Config) password() string { return fromEnv(c.Account.PasswordEnv, c.Account.Password) }
func (c Config) apiToken() string { return fromEnv(c.API.TokenEnv, c.API.Token) }

// loadConfig reads <dir>/config.yaml and resolves relative paths against dir.
func loadConfig(dir string) (Config, error) {
	c := defaultConfig()
	file, err := os.Open(filepath.Join(dir, "config.yaml"))
	if err != nil {
		return c, err
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&c); err != nil {
		return c, fmt.Errorf("decode config: %w", err)
	}
	if err := c.validate(); err != nil {
		return c, err
	}
	if !filepath.IsAbs(c.Account.TokenPath) {
		c.Account.TokenPath = filepath.Join(dir, c.Account.TokenPath)
	}
	if !filepath.IsAbs(c.Database.Path) {
		c.Database.Path = filepath.Join(dir, c.Database.Path)
	}
	return c, nil
}

func (c Config) validate() error {
	switch {
	case strings.TrimSpace(c.Account.TokenPath) == "":
		return errors.New("account.token_path cannot be empty")
	case strings.TrimSpace(c.API.Listen) == "":
		return errors.New("api.listen cannot be empty")
	case strings.TrimSpace(c.Database.Path) == "":
		return errors.New("database.path cannot be empty")
	case c.Cache.DevicesTTL < 0 || c.Cache.SpecTTL < 0:
		return errors.New("cache TTLs cannot be negative")
	}
	return nil
}
