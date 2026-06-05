package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigYAML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "apk-config.yml")
	data := []byte(`
input_apk: app-release.apk
final_output: dist/app-protected.apk
work_dir: build/temp
keep_work_dir: true
protections:
  enabled: true
  random_package: true
  package_prefix: com.secure.guard
  multi_dex_encrypt: true
  compress_before_encrypt: true
  encryption_secret: mx-guard-v1
signing:
  enabled: true
  apksigner_path: tools/apksigner
  keystore: sign/release.keystore
  key_alias: release
  store_pass: secret
zipalign:
  enabled: true
  path: tools/zipalign
reporting:
  json: reports/run.json
`)
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.InputAPK != "app-release.apk" {
		t.Fatalf("InputAPK = %q, want app-release.apk", cfg.InputAPK)
	}
	if !cfg.KeepWorkDir {
		t.Fatal("KeepWorkDir = false, want true")
	}
	if !cfg.Protections.Enabled || !cfg.Protections.RandomPackage || !cfg.Protections.MultiDexEncrypt {
		t.Fatalf("Protections not loaded from YAML: %+v", cfg.Protections)
	}
	if cfg.Signing.KeyAlias != "release" {
		t.Fatalf("Signing.KeyAlias = %q, want release", cfg.Signing.KeyAlias)
	}
	if cfg.Reporting.JSON != "reports/run.json" {
		t.Fatalf("Reporting.JSON = %q, want reports/run.json", cfg.Reporting.JSON)
	}
}

func TestLoadConfigJSON(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "apk-config.json")
	data := []byte(`{
		"input_apk": "app-release.apk",
		"final_output": "dist/app-protected.apk",
		"signing": {
			"enabled": true,
			"keystore": "sign/release.keystore",
			"key_alias": "release",
			"store_pass": "secret"
		}
	}`)
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.InputAPK != "app-release.apk" {
		t.Fatalf("InputAPK = %q, want app-release.apk", cfg.InputAPK)
	}
	if cfg.Signing.Keystore != "sign/release.keystore" {
		t.Fatalf("Signing.Keystore = %q, want sign/release.keystore", cfg.Signing.Keystore)
	}
}

func TestLoadConfigYAMLFallbackForNonYAMLExtension(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "apk-config.conf")
	data := []byte(`
input_apk: app-release.apk
protections:
  enabled: true
  pseudo_encrypt: true
`)
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.InputAPK != "app-release.apk" {
		t.Fatalf("InputAPK = %q, want app-release.apk", cfg.InputAPK)
	}
	if !cfg.Protections.PseudoEncrypt {
		t.Fatal("Protections.PseudoEncrypt = false, want true")
	}
}

func TestConfigFinalizeResolvesPathsFromConfigDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, "configs")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	apkPath := filepath.Join(cfgDir, "app-release.apk")
	if err := os.WriteFile(apkPath, []byte("apk"), 0o644); err != nil {
		t.Fatalf("write apk: %v", err)
	}
	cfgPath := filepath.Join(cfgDir, "apk-config.yml")
	data := []byte(`
input_apk: app-release.apk
final_output: dist/app-protected.apk
work_dir: build/temp
signing:
  enabled: true
  keystore: sign/release.keystore
  key_alias: release
  store_pass: secret
`)
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if err := cfg.finalize(tmpDir); err != nil {
		t.Fatalf("finalize() error = %v", err)
	}

	wantInput := filepath.Join(cfgDir, "app-release.apk")
	if cfg.InputAPK != wantInput {
		t.Fatalf("InputAPK = %q, want %q", cfg.InputAPK, wantInput)
	}
	wantOutput := filepath.Join(cfgDir, "dist", "app-protected.apk")
	if cfg.FinalOutput != wantOutput {
		t.Fatalf("FinalOutput = %q, want %q", cfg.FinalOutput, wantOutput)
	}
	wantKeystore := filepath.Join(cfgDir, "sign", "release.keystore")
	if cfg.Signing.Keystore != wantKeystore {
		t.Fatalf("Signing.Keystore = %q, want %q", cfg.Signing.Keystore, wantKeystore)
	}
}

func TestConfigFinalizePreservesExplicitScanningDisabled(t *testing.T) {
	cfg := &Config{
		InputAPK: "app-release.apk",
		Scanning: ScanningConfig{
			Enabled: false,
		},
	}

	if err := cfg.finalize(t.TempDir()); err != nil {
		t.Fatalf("finalize() error = %v", err)
	}

	if cfg.Scanning.Enabled {
		t.Fatal("Scanning.Enabled = true, want explicit false to be preserved")
	}
}
