package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadExpandsEnvRefs(t *testing.T) {
	t.Setenv("GOPROTECT_TEST_KEY", "deadbeef1234")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := "input: module.bc\nvmp:\n  static_key: \"${GOPROTECT_TEST_KEY}\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.VMP.StaticKey != "deadbeef1234" {
		t.Fatalf("static_key = %q, want expanded value", cfg.VMP.StaticKey)
	}
}

func TestLoadMissingEnvRefFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := "input: module.bc\nvmp:\n  static_key: \"${GOPROTECT_TEST_UNSET_KEY}\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("want error for unset environment reference")
	}
	if !strings.Contains(err.Error(), "GOPROTECT_TEST_UNSET_KEY") {
		t.Fatalf("error should name the missing variable, got: %v", err)
	}
}
