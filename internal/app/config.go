package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Ero-Cat/Apk-Protector-Tool/internal/envref"
)

// Config controls the reinforcement and signing workflow.
type Config struct {
	InputAPK     string             `json:"input_apk" yaml:"input_apk"`
	FinalOutput  string             `json:"final_output" yaml:"final_output"`
	WorkDir      string             `json:"work_dir" yaml:"work_dir"`
	KeepWorkDir  bool               `json:"keep_work_dir" yaml:"keep_work_dir"`
	Protections  ProtectionConfig   `json:"protections" yaml:"protections"`
	Scanning     ScanningConfig     `json:"scanning" yaml:"scanning"`
	Reinforce    ReinforceConfig    `json:"reinforce" yaml:"reinforce"`
	Zipalign     ZipalignConfig     `json:"zipalign" yaml:"zipalign"`
	Signing      SigningConfig      `json:"signing" yaml:"signing"`
	Verification VerificationConfig `json:"verification" yaml:"verification"`
	Reporting    ReportingConfig    `json:"reporting" yaml:"reporting"`

	baseDir string
}

// ReinforceConfig defines optional third-party hardening command execution.
type ReinforceConfig struct {
	Enabled            bool              `json:"enabled" yaml:"enabled"`
	Command            string            `json:"command" yaml:"command"`
	Args               []string          `json:"args" yaml:"args"`
	Env                map[string]string `json:"env" yaml:"env"`
	Timeout            string            `json:"timeout" yaml:"timeout"`
	OutputAPK          string            `json:"output_apk" yaml:"output_apk"`
	SkipIfOutputExists bool              `json:"skip_if_output_exists" yaml:"skip_if_output_exists"`
}

// ZipalignConfig runs Android's zipalign utility before signing.
type ZipalignConfig struct {
	Enabled   bool   `json:"enabled" yaml:"enabled"`
	Path      string `json:"path" yaml:"path"`
	Alignment int    `json:"alignment" yaml:"alignment"`
	OutputAPK string `json:"output_apk" yaml:"output_apk"`
}

// SigningConfig holds the V1+V2 signing parameters.
type SigningConfig struct {
	Enabled           bool     `json:"enabled" yaml:"enabled"`
	ApksignerPath     string   `json:"apksigner_path" yaml:"apksigner_path"`
	Keystore          string   `json:"keystore" yaml:"keystore"`
	KeyAlias          string   `json:"key_alias" yaml:"key_alias"`
	StorePass         string   `json:"store_pass" yaml:"store_pass"`
	KeyPass           string   `json:"key_pass" yaml:"key_pass"`
	OutputAPK         string   `json:"output_apk" yaml:"output_apk"`
	CreateKeystore    bool     `json:"create_keystore" yaml:"create_keystore"`
	KeytoolPath       string   `json:"keytool_path" yaml:"keytool_path"`
	KeyAlgorithm      string   `json:"key_algorithm" yaml:"key_algorithm"`
	KeySize           int      `json:"key_size" yaml:"key_size"`
	ValidityDays      int      `json:"validity_days" yaml:"validity_days"`
	DistinguishedName string   `json:"distinguished_name" yaml:"distinguished_name"`
	AdditionalArgs    []string `json:"additional_args" yaml:"additional_args"`
}

// VerificationConfig controls optional apksigner verify execution.
type VerificationConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
}

// ReportingConfig controls structured report emission.
type ReportingConfig struct {
	JSON string `json:"json" yaml:"json"`
}

// ProtectionConfig configures APK-level transformations.
type ProtectionConfig struct {
	Enabled               bool   `json:"enabled" yaml:"enabled"`
	RandomPackage         bool   `json:"random_package" yaml:"random_package"`
	PackagePrefix         string `json:"package_prefix" yaml:"package_prefix"`
	DexEncrypt            bool   `json:"dex_encrypt" yaml:"dex_encrypt"`
	MultiDexEncrypt       bool   `json:"multi_dex_encrypt" yaml:"multi_dex_encrypt"`
	PseudoEncrypt         bool   `json:"pseudo_encrypt" yaml:"pseudo_encrypt"`
	CompressBeforeEncrypt bool   `json:"compress_before_encrypt" yaml:"compress_before_encrypt"`
	EncryptionSecret      string `json:"encryption_secret" yaml:"encryption_secret"`
}

// ScanningConfig controls APK feature scanning.
type ScanningConfig struct {
	Enabled       bool     `json:"enabled" yaml:"enabled"`
	ApksignerPath string   `json:"apksigner_path" yaml:"apksigner_path"`
	Keywords      []string `json:"keywords" yaml:"keywords"`
	KeyLeakHints  []string `json:"key_leak_hints" yaml:"key_leak_hints"`
	MaxScanSizeMB int      `json:"max_scan_size_mb" yaml:"max_scan_size_mb"`
}

// LoadConfig reads an optional YAML/JSON file. When path is empty an empty config is returned.
func LoadConfig(path string) (*Config, error) {
	cfg := &Config{
		Scanning: ScanningConfig{
			Enabled: true,
		},
	}
	if path == "" {
		return cfg, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := unmarshalConfig(data, absPath, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.expandEnvRefs(); err != nil {
		return nil, fmt.Errorf("config %s: %w", absPath, err)
	}

	cfg.baseDir = filepath.Dir(absPath)
	return cfg, nil
}

// expandEnvRefs resolves ${VAR} and ${VAR:-default} references in path and
// secret fields. Unset variables without a default are errors so that a
// missing secret fails at load time instead of being used literally.
func (c *Config) expandEnvRefs() error {
	fields := []*string{
		&c.InputAPK,
		&c.FinalOutput,
		&c.WorkDir,
		&c.Protections.PackagePrefix,
		&c.Protections.EncryptionSecret,
		&c.Reinforce.Command,
		&c.Reinforce.OutputAPK,
		&c.Zipalign.Path,
		&c.Zipalign.OutputAPK,
		&c.Signing.ApksignerPath,
		&c.Signing.Keystore,
		&c.Signing.KeyAlias,
		&c.Signing.StorePass,
		&c.Signing.KeyPass,
		&c.Signing.OutputAPK,
		&c.Signing.KeytoolPath,
		&c.Signing.DistinguishedName,
		&c.Reporting.JSON,
		&c.Scanning.ApksignerPath,
	}
	for _, f := range fields {
		if !strings.Contains(*f, "${") {
			continue
		}
		expanded, err := envref.Expand(*f)
		if err != nil {
			return err
		}
		*f = expanded
	}
	return nil
}

func unmarshalConfig(data []byte, path string, cfg *Config) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yml", ".yaml":
		return yaml.Unmarshal(data, cfg)
	default:
		if err := json.Unmarshal(data, cfg); err != nil {
			return yaml.Unmarshal(data, cfg)
		}
		return nil
	}
}

// resolvePaths converts relative paths into absolute ones. Relative paths are resolved
// against the config location (when available) or the provided working directory.
func (c *Config) resolvePaths(cwd string) error {
	resolve := func(value string, force bool) string {
		if value == "" {
			return ""
		}
		if value == "~" || strings.HasPrefix(value, "~/") || strings.HasPrefix(value, "~\\") {
			if home, err := os.UserHomeDir(); err == nil {
				trimmed := strings.TrimPrefix(value, "~")
				trimmed = strings.TrimPrefix(trimmed, "/")
				trimmed = strings.TrimPrefix(trimmed, "\\")
				value = filepath.Join(home, trimmed)
			}
		}
		if filepath.IsAbs(value) {
			return value
		}
		if !force {
			if !strings.ContainsRune(value, os.PathSeparator) && !strings.ContainsRune(value, '\\') && !strings.ContainsRune(value, '/') {
				return value
			}
		}
		base := c.baseDir
		if base == "" {
			base = cwd
		}
		if base == "" {
			return value
		}
		return filepath.Join(base, value)
	}

	c.InputAPK = resolve(c.InputAPK, true)
	c.FinalOutput = resolve(c.FinalOutput, true)
	c.WorkDir = resolve(c.WorkDir, true)
	c.Reinforce.OutputAPK = resolve(c.Reinforce.OutputAPK, true)
	c.Zipalign.Path = resolve(c.Zipalign.Path, false)
	c.Zipalign.OutputAPK = resolve(c.Zipalign.OutputAPK, true)
	c.Signing.ApksignerPath = resolve(c.Signing.ApksignerPath, false)
	c.Signing.Keystore = resolve(c.Signing.Keystore, true)
	c.Signing.OutputAPK = resolve(c.Signing.OutputAPK, true)
	c.Signing.KeytoolPath = resolve(c.Signing.KeytoolPath, false)
	c.Reporting.JSON = resolve(c.Reporting.JSON, true)
	c.Scanning.ApksignerPath = resolve(c.Scanning.ApksignerPath, false)
	return nil
}

func (c *Config) finalize(cwd string) error {
	if err := c.resolvePaths(cwd); err != nil {
		return err
	}

	if !c.Protections.Enabled {
		if c.Protections.RandomPackage || c.Protections.DexEncrypt || c.Protections.MultiDexEncrypt || c.Protections.PseudoEncrypt || c.Protections.CompressBeforeEncrypt {
			c.Protections.Enabled = true
		}
	}
	if c.Protections.PackagePrefix == "" {
		c.Protections.PackagePrefix = "com.protector"
	}
	if c.Scanning.Enabled {
		if c.Scanning.ApksignerPath == "" {
			c.Scanning.ApksignerPath = c.Signing.ApksignerPath
		}
		if c.Scanning.ApksignerPath == "" {
			c.Scanning.ApksignerPath = "apksigner"
		}
		if c.Scanning.MaxScanSizeMB <= 0 {
			c.Scanning.MaxScanSizeMB = 4
		}
		if len(c.Scanning.Keywords) == 0 {
			c.Scanning.Keywords = []string{"root", "frida", "xposed", "magisk", "genymotion", "proxy", "vpn", "emulator"}
		}
		if len(c.Scanning.KeyLeakHints) == 0 {
			c.Scanning.KeyLeakHints = []string{"BEGIN PRIVATE KEY", "AKIA", "SECRET", "password=", "access_key", "token"}
		}
	}

	if c.Signing.Enabled == false && (c.Signing.Keystore != "" || c.Signing.KeyAlias != "" || c.Signing.ApksignerPath != "" || c.Signing.OutputAPK != "") {
		c.Signing.Enabled = true
	}

	if c.Zipalign.Alignment == 0 {
		c.Zipalign.Alignment = 4
	}
	if c.Signing.KeyAlgorithm == "" {
		c.Signing.KeyAlgorithm = "RSA"
	}
	if c.Signing.KeySize == 0 {
		c.Signing.KeySize = 2048
	}
	if c.Signing.ValidityDays == 0 {
		c.Signing.ValidityDays = 3650
	}
	if c.Signing.DistinguishedName == "" {
		c.Signing.DistinguishedName = "CN=Android Hardening,O=Automation,OU=Security,L=Unknown,ST=Unknown,C=CN"
	}
	if c.Signing.ApksignerPath == "" {
		c.Signing.ApksignerPath = "apksigner"
	}
	if c.Zipalign.Path == "" {
		c.Zipalign.Path = "zipalign"
	}
	if c.Signing.KeytoolPath == "" {
		c.Signing.KeytoolPath = "keytool"
	}
	if c.Signing.KeyPass == "" {
		c.Signing.KeyPass = c.Signing.StorePass
	}
	if c.InputAPK == "" {
		return fmt.Errorf("input_apk is required")
	}
	if c.Signing.Enabled {
		if c.Signing.Keystore == "" {
			return fmt.Errorf("signing.keystore is required when signing is enabled")
		}
		if c.Signing.KeyAlias == "" {
			return fmt.Errorf("signing.key_alias is required when signing is enabled")
		}
		if c.Signing.StorePass == "" {
			return fmt.Errorf("signing.store_pass is required when signing is enabled")
		}
	}
	return nil
}
