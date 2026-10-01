package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ero-Cat/Apk-Protector-Tool/internal/app"
)

// parseWithFlags 走真实的 FlagSet 解析路径，返回解析后的 options 与
// "用户显式传入的 flag 名"集合（与 execute 的采集方式一致）。
func parseWithFlags(t *testing.T, args ...string) (*cliOptions, map[string]bool) {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(&strings.Builder{})
	opts := registerFlags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse %v: %v", args, err)
	}
	explicit := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	return opts, explicit
}

func TestForceModeScan(t *testing.T) {
	cfg := &app.Config{}
	cfg.Protections.Enabled = true
	cfg.Protections.MultiDexEncrypt = true
	cfg.Signing.Enabled = true
	cfg.Zipalign.Enabled = true
	cfg.Verification.Enabled = true
	cfg.Reinforce.Enabled = true

	forceMode("scan", cfg)

	if !cfg.Scanning.Enabled {
		t.Error("scan must force scanning on")
	}
	if cfg.Protections.Enabled || cfg.Signing.Enabled || cfg.Zipalign.Enabled ||
		cfg.Verification.Enabled || cfg.Reinforce.Enabled {
		t.Error("scan must force every other stage off")
	}
}

func TestForceModeSign(t *testing.T) {
	cfg := &app.Config{}
	cfg.Scanning.Enabled = true
	cfg.Protections.Enabled = true
	cfg.Verification.Enabled = true

	forceMode("sign", cfg)

	if cfg.Scanning.Enabled || cfg.Protections.Enabled {
		t.Error("sign must force scanning and protections off")
	}
	if !cfg.Verification.Enabled {
		t.Error("sign must keep verification as configured")
	}
}

// TestExplicitProtectFalseBeatsConfig 验证显式 -protect=false 清掉配置启用的
// 子开关（否则 finalize 会隐式复活保护）。
func TestExplicitProtectFalseBeatsConfig(t *testing.T) {
	cfg := &app.Config{}
	cfg.Protections.Enabled = true
	cfg.Protections.MultiDexEncrypt = true
	cfg.Protections.RandomPackage = true

	opts, explicit := parseWithFlags(t, "-protect=false")
	if err := applyOverrides(cfg, opts, explicit); err != nil {
		t.Fatal(err)
	}

	if cfg.Protections.Enabled {
		t.Fatal("explicit -protect=false must disable protections")
	}
	if cfg.Protections.MultiDexEncrypt || cfg.Protections.RandomPackage {
		t.Fatal("non-explicit sub-toggles must be cleared so finalize cannot resurrect protections")
	}
}

// TestExplicitSubToggleSurvivesDisable 验证显式传入的子开关在 -protect=false
// 下仍按用户取值应用。
func TestExplicitSubToggleSurvivesDisable(t *testing.T) {
	cfg := &app.Config{}
	cfg.Protections.Enabled = true
	cfg.Protections.RandomPackage = true

	opts, explicit := parseWithFlags(t, "-protect=false", "-protect-random-package")
	if err := applyOverrides(cfg, opts, explicit); err != nil {
		t.Fatal(err)
	}
	if cfg.Protections.Enabled {
		t.Fatal("protections stay disabled")
	}
	if !cfg.Protections.RandomPackage {
		t.Fatal("explicit -protect-random-package (true) must survive")
	}
}

func TestUnsetEnvFlagErrors(t *testing.T) {
	opts, explicit := parseWithFlags(t, "-store-pass-env", "PROTECTOR_TEST_UNSET")
	err := applyOverrides(&app.Config{}, opts, explicit)
	if err == nil || !strings.Contains(err.Error(), "PROTECTOR_TEST_UNSET") {
		t.Fatalf("want actionable error naming the variable, got: %v", err)
	}
}

func TestEnvFlagResolution(t *testing.T) {
	t.Setenv("PROTECTOR_TEST_PASS", "sekrit")
	opts, explicit := parseWithFlags(t, "-store-pass-env", "PROTECTOR_TEST_PASS")
	cfg := &app.Config{}
	if err := applyOverrides(cfg, opts, explicit); err != nil {
		t.Fatal(err)
	}
	if cfg.Signing.StorePass != "sekrit" {
		t.Fatalf("store_pass = %q, want resolved env value", cfg.Signing.StorePass)
	}
}

// TestConfigTemplateLoads 验证 config init 模板能被 app.LoadConfig 消费
// （含 ${VAR} 引用的展开）。
func TestConfigTemplateLoads(t *testing.T) {
	t.Setenv("APK_PROTECT_SECRET", "tpl-secret")
	t.Setenv("APK_STORE_PASS", "tpl-pass")

	path := filepath.Join(t.TempDir(), "protector.config.yml")
	if err := writeTemplate(path); err != nil {
		t.Fatal(err)
	}
	cfg, err := app.LoadConfig(path)
	if err != nil {
		t.Fatalf("template must load: %v", err)
	}
	if cfg.Protections.EncryptionSecret != "tpl-secret" {
		t.Fatalf("encryption_secret = %q, want expanded env value", cfg.Protections.EncryptionSecret)
	}
	if cfg.Signing.StorePass != "tpl-pass" {
		t.Fatalf("store_pass = %q, want expanded env value", cfg.Signing.StorePass)
	}
	if !cfg.Signing.Enabled || !cfg.Protections.Enabled {
		t.Fatal("template defaults should enable signing and protections")
	}
}

// writeTemplate 是 configInit 的可测内核（避免测试真的写 cwd 文件）。
func writeTemplate(path string) error {
	return os.WriteFile(path, []byte(configTemplate), 0o644)
}
