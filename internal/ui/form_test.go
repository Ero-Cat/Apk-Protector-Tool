package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ero-Cat/Apk-Protector-Tool/internal/presets"
)

func TestFieldsFor(t *testing.T) {
	quick := FieldsFor(presets.Quick)
	full := FieldsFor(presets.Full)

	if len(full) <= len(quick) {
		t.Fatalf("full (%d fields) should have more fields than quick (%d)", len(full), len(quick))
	}
	for _, key := range []string{keySecretEnv, keyMultiDex, keyPseudo} {
		found := false
		for _, spec := range full {
			if spec.Key == key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("full profile is missing field %q", key)
		}
	}
	for _, key := range []string{keySecretEnv, keyMultiDex} {
		for _, spec := range quick {
			if spec.Key == key {
				t.Errorf("quick profile should not contain field %q", key)
			}
		}
	}
}

func newValidForm(t *testing.T, profile presets.Profile) *Form {
	t.Helper()
	t.Setenv("UI_TEST_STORE_PASS", "store-secret")
	t.Setenv("UI_TEST_SECRET", "dex-secret")

	dir := t.TempDir()
	apk := filepath.Join(dir, "app.apk")
	if err := os.WriteFile(apk, []byte("PK\x03\x04fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	keystore := filepath.Join(dir, "release.keystore")
	if err := os.WriteFile(keystore, []byte("ks"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Fake build-tools binaries so KindExec validation passes without an
	// Android SDK on the test machine.
	zipalign := filepath.Join(dir, "zipalign")
	apksigner := filepath.Join(dir, "apksigner")
	for _, bin := range []string{zipalign, apksigner} {
		if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	f := NewForm(profile, apk)
	f.Values[keyZipalign] = zipalign
	f.Values[keyApksigner] = apksigner
	f.Values[keyKeystore] = keystore
	f.Values[keyAlias] = "release"
	f.Values[keyStorePassEnv] = "UI_TEST_STORE_PASS"
	if profile == presets.Full {
		f.Values[keySecretEnv] = "UI_TEST_SECRET"
	}
	return f
}

func TestValidateAcceptsMinimalForm(t *testing.T) {
	f := newValidForm(t, presets.Quick)
	if errs := f.Validate(); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestValidateRejectsUnsetEnvVar(t *testing.T) {
	f := newValidForm(t, presets.Quick)
	f.Values[keyStorePassEnv] = "UI_TEST_NOPE"
	errs := f.Validate()
	if len(errs) != 1 || !strings.Contains(errs[0], "UI_TEST_NOPE") {
		t.Fatalf("want single error naming the variable, got: %v", errs)
	}
}

func TestValidateRejectsBadEnvName(t *testing.T) {
	f := newValidForm(t, presets.Quick)
	f.Values[keyStorePassEnv] = "not a name"
	if errs := f.Validate(); len(errs) == 0 {
		t.Fatal("want error for invalid variable name")
	}
}

func TestValidateKeystoreMissingWithoutAutoCreate(t *testing.T) {
	f := newValidForm(t, presets.Quick)
	f.Values[keyKeystore] = filepath.Join(t.TempDir(), "nope.keystore")
	if errs := f.Validate(); len(errs) == 0 {
		t.Fatal("want error for missing keystore")
	}

	f.Toggles[keyCreateKS] = true
	if errs := f.Validate(); len(errs) != 0 {
		t.Fatalf("auto-generation should tolerate missing keystore, got: %v", errs)
	}
}

func TestValidateRejectsMissingExecPath(t *testing.T) {
	f := newValidForm(t, presets.Quick)
	f.Values[keyZipalign] = "/definitely/not/a/real/zipalign"
	errs := f.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "zipalign") {
			found = true
		}
	}
	if !found {
		t.Fatalf("want error about missing zipalign, got: %v", errs)
	}
}

func TestToConfigResolvesSecretsFromEnv(t *testing.T) {
	f := newValidForm(t, presets.Full)
	f.Values[keyPrefix] = "com.example.hardened"
	f.Toggles[keyPseudo] = false

	cfg, err := f.ToConfig()
	if err != nil {
		t.Fatalf("ToConfig: %v", err)
	}
	if cfg.Signing.StorePass != "store-secret" {
		t.Fatalf("store_pass = %q, want resolved env value", cfg.Signing.StorePass)
	}
	if cfg.Signing.KeyPass != "store-secret" {
		t.Fatalf("key_pass should fall back to store pass, got %q", cfg.Signing.KeyPass)
	}
	if cfg.Protections.EncryptionSecret != "dex-secret" {
		t.Fatalf("encryption_secret = %q, want resolved env value", cfg.Protections.EncryptionSecret)
	}
	if cfg.Protections.PackagePrefix != "com.example.hardened" {
		t.Fatalf("package_prefix = %q", cfg.Protections.PackagePrefix)
	}
	if cfg.Protections.PseudoEncrypt {
		t.Fatal("pseudo toggle was turned off in the form but stayed enabled")
	}
	if !cfg.Protections.MultiDexEncrypt || !cfg.Protections.RandomPackage {
		t.Fatal("full profile toggles should survive into the config")
	}
	if cfg.Zipalign.Path == "" {
		t.Fatal("zipalign path should default to a bare name")
	}
}

func TestPreviewJSONHidesSecretValues(t *testing.T) {
	f := newValidForm(t, presets.Full)

	preview, err := f.PreviewJSON()
	if err != nil {
		t.Fatalf("PreviewJSON: %v", err)
	}
	if strings.Contains(preview, "store-secret") || strings.Contains(preview, "dex-secret") {
		t.Fatal("preview leaks plain-text secret values")
	}
	for _, ref := range []string{"${UI_TEST_STORE_PASS}", "${UI_TEST_SECRET}"} {
		if !strings.Contains(preview, ref) {
			t.Fatalf("preview should contain %s", ref)
		}
	}
}

func TestDefaultOutputPath(t *testing.T) {
	got := DefaultOutputPath("/tmp/app-release.apk")
	if want := filepath.Join("dist", "app-release-protected.apk"); got != want {
		t.Fatalf("DefaultOutputPath = %q, want %q", got, want)
	}
}
