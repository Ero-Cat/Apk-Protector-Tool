package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigExpandsEnvRefs(t *testing.T) {
	t.Setenv("CFG_TEST_SECRET", "s3cr3t-value")
	t.Setenv("CFG_TEST_TOOLS", "/opt/build-tools/36.1.0")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	content := `{
  "input_apk": "app.apk",
  "protections": {
    "enabled": true,
    "encryption_secret": "${CFG_TEST_SECRET}"
  },
  "zipalign": {
    "path": "${CFG_TEST_TOOLS}/zipalign"
  },
  "signing": {
    "store_pass": "${CFG_TEST_SECRET}"
  }
}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Protections.EncryptionSecret != "s3cr3t-value" {
		t.Fatalf("encryption_secret = %q, want expanded value", cfg.Protections.EncryptionSecret)
	}
	if cfg.Zipalign.Path != "/opt/build-tools/36.1.0/zipalign" {
		t.Fatalf("zipalign path = %q, want expanded path", cfg.Zipalign.Path)
	}
	if cfg.Signing.StorePass != "s3cr3t-value" {
		t.Fatalf("store_pass = %q, want expanded value", cfg.Signing.StorePass)
	}
}

func TestLoadConfigEnvRefWithDefault(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yml")
	content := "protections:\n  encryption_secret: ${CFG_TEST_UNSET:-fallback-secret}\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if want := "fallback-secret"; cfg.Protections.EncryptionSecret != want {
		t.Fatalf("encryption_secret = %q, want %q", cfg.Protections.EncryptionSecret, want)
	}
}

func TestLoadConfigMissingEnvRefFails(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	content := `{"signing": {"store_pass": "${CFG_TEST_DEFINITELY_UNSET}"}}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("want error for unset environment reference")
	}
	if !strings.Contains(err.Error(), "CFG_TEST_DEFINITELY_UNSET") {
		t.Fatalf("error should name the missing variable, got: %v", err)
	}
}
