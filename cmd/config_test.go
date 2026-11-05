package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSampleConfigLoads(t *testing.T) {
	config, err := loadConfig("..")
	if err != nil {
		t.Fatal(err)
	}
	if config.API.Listen != "127.0.0.1:8080" || config.Cache.DevicesTTL != time.Minute {
		t.Fatalf("config = %+v", config)
	}
	if config.Account.Password != "" || config.API.Token != "" {
		t.Fatal("the sample config must not hold secrets")
	}
}

func TestPathsAreRelativeToTheDataDirectory(t *testing.T) {
	dir := writeConfig(t, "database:\n  path: custom.db\naccount:\n  token_path: /abs/token\ncache:\n  spec_ttl: 2h\n")
	config, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if config.Database.Path != filepath.Join(dir, "custom.db") || config.Account.TokenPath != "/abs/token" {
		t.Fatalf("config = %+v", config)
	}
	if config.Cache.SpecTTL != 2*time.Hour || config.Cache.DevicesTTL != time.Minute {
		t.Fatalf("cache = %+v", config.Cache)
	}
}

func TestDefaultPathsAreInTheDataDirectory(t *testing.T) {
	dir := writeConfig(t, "{}\n")
	config, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if config.Database.Path != filepath.Join(dir, "mihome.db") || config.Account.TokenPath != filepath.Join(dir, "mi.token") {
		t.Fatalf("config = %+v", config)
	}
}

func TestEnvironmentWinsOverTheFile(t *testing.T) {
	dir := writeConfig(t, "account:\n  username: file-user\n  password: file-pass\n  password_env: MI_TEST_PASSWORD\n  username_env: MI_TEST_USERNAME\napi:\n  token: file-token\n  token_env: MI_TEST_TOKEN\n")
	config, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MI_TEST_USERNAME", "")
	t.Setenv("MI_TEST_PASSWORD", "env-pass")
	t.Setenv("MI_TEST_TOKEN", "env-token")
	if config.username() != "file-user" || config.password() != "env-pass" || config.apiToken() != "env-token" {
		t.Fatalf("%q %q %q", config.username(), config.password(), config.apiToken())
	}
}

func TestInvalidConfigs(t *testing.T) {
	for name, content := range map[string]string{
		"unknown field": "nope: 1\n",
		"empty listen":  "api:\n  listen: \"\"\n",
		"empty db":      "database:\n  path: \"\"\n",
		"negative ttl":  "cache:\n  devices_ttl: -1s\n",
	} {
		if _, err := loadConfig(writeConfig(t, content)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := loadConfig(t.TempDir()); err == nil {
		t.Error("missing file: expected an error")
	}
}

func TestSpecLanguageConfiguration(t *testing.T) {
	t.Setenv("MI_SPEC_LANGUAGE", "")
	config, err := loadConfig(writeConfig(t, "{}\n"))
	if err != nil || config.specLanguage() != "zh_cn" {
		t.Fatalf("default language = %q, error = %v", config.specLanguage(), err)
	}
	config, err = loadConfig(writeConfig(t, "spec:\n  language: en\n"))
	if err != nil || config.specLanguage() != "en" {
		t.Fatalf("configured language = %q, error = %v", config.specLanguage(), err)
	}
	t.Setenv("MI_SPEC_LANGUAGE", "zh_tw")
	if config.specLanguage() != "zh_tw" {
		t.Fatal("environment must override the file")
	}
	config.Spec.LanguageEnv = "MI_TEST_SPEC_LANGUAGE"
	t.Setenv("MI_TEST_SPEC_LANGUAGE", "ja")
	if config.specLanguage() != "ja" {
		t.Fatal("custom environment variable was ignored")
	}
	config.Spec.LanguageEnv = ""
	if config.specLanguage() != "en" {
		t.Fatal("empty language_env must disable the environment override")
	}
}

func TestLoadEnvKeepsExistingVariables(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "# comment\nexport MI_TEST_A=\"quoted\"\nMI_TEST_B=file\nnot a variable\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MI_TEST_A", "")
	os.Unsetenv("MI_TEST_A")
	t.Setenv("MI_TEST_B", "shell")
	if err := loadEnv(path); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv("MI_TEST_A")
	if os.Getenv("MI_TEST_A") != "quoted" || os.Getenv("MI_TEST_B") != "shell" {
		t.Fatalf("A=%q B=%q", os.Getenv("MI_TEST_A"), os.Getenv("MI_TEST_B"))
	}
	if err := loadEnv(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatal(err)
	}
}
