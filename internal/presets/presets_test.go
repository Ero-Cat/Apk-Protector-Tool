package presets

import (
	"testing"

	"github.com/Ero-Cat/Apk-Protector-Tool/internal/app"
)

func TestParse(t *testing.T) {
	for _, ok := range []string{"quick", "full", "sign-only"} {
		if _, err := Parse(ok); err != nil {
			t.Fatalf("Parse(%q) error: %v", ok, err)
		}
	}
	if _, err := Parse("turbo"); err == nil {
		t.Fatal("Parse(turbo) should fail")
	}
}

func TestApplySwitchMatrix(t *testing.T) {
	tests := []struct {
		profile                            Profile
		scan, protect, align, sign, verify bool
	}{
		{Quick, true, false, true, true, false},
		{Full, true, true, true, true, true},
		{SignOnly, false, false, true, true, true},
	}
	for _, tt := range tests {
		t.Run(string(tt.profile), func(t *testing.T) {
			cfg := &app.Config{}
			Apply(tt.profile, cfg)
			if cfg.Scanning.Enabled != tt.scan {
				t.Errorf("scanning = %v, want %v", cfg.Scanning.Enabled, tt.scan)
			}
			if cfg.Protections.Enabled != tt.protect {
				t.Errorf("protections = %v, want %v", cfg.Protections.Enabled, tt.protect)
			}
			if cfg.Zipalign.Enabled != tt.align {
				t.Errorf("zipalign = %v, want %v", cfg.Zipalign.Enabled, tt.align)
			}
			if cfg.Signing.Enabled != tt.sign {
				t.Errorf("signing = %v, want %v", cfg.Signing.Enabled, tt.sign)
			}
			if cfg.Verification.Enabled != tt.verify {
				t.Errorf("verification = %v, want %v", cfg.Verification.Enabled, tt.verify)
			}
		})
	}
}

func TestApplyFullEnablesProtectionToggles(t *testing.T) {
	cfg := &app.Config{}
	Apply(Full, cfg)
	if !cfg.Protections.MultiDexEncrypt || !cfg.Protections.CompressBeforeEncrypt ||
		!cfg.Protections.RandomPackage || !cfg.Protections.PseudoEncrypt {
		t.Fatal("full profile should enable multi-dex, compress, random package and pseudo")
	}
}

func TestApplyKeepsSecretsAndPaths(t *testing.T) {
	cfg := &app.Config{}
	cfg.Protections.EncryptionSecret = "keep-me"
	cfg.Protections.PackagePrefix = "com.example"
	cfg.Signing.Keystore = "/tmp/release.keystore"

	Apply(SignOnly, cfg) // disables protections
	if cfg.Protections.EncryptionSecret != "keep-me" {
		t.Fatalf("encryption secret was erased: %q", cfg.Protections.EncryptionSecret)
	}
	if cfg.Protections.PackagePrefix != "com.example" {
		t.Fatalf("package prefix was erased: %q", cfg.Protections.PackagePrefix)
	}
	if cfg.Signing.Keystore != "/tmp/release.keystore" {
		t.Fatalf("keystore was erased: %q", cfg.Signing.Keystore)
	}

	Apply(Full, cfg) // re-enables protections
	if cfg.Protections.EncryptionSecret != "keep-me" {
		t.Fatalf("full profile should keep the existing secret, got %q", cfg.Protections.EncryptionSecret)
	}
}

func TestApplyUnknownProfileIsNoop(t *testing.T) {
	cfg := &app.Config{}
	cfg.Scanning.Enabled = true
	Apply("nope", cfg)
	if !cfg.Scanning.Enabled {
		t.Fatal("unknown profile must not modify the config")
	}
}
