// Package ui implements the interactive `protector ui` wizard.
//
// form.go holds the pure form logic — field definitions, validation and the
// mapping onto app.Config — deliberately free of any terminal dependency so
// it stays table-testable.
package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Ero-Cat/Apk-Protector-Tool/internal/app"
	"github.com/Ero-Cat/Apk-Protector-Tool/internal/presets"
)

// FieldKind describes how a form field is rendered and validated.
type FieldKind int

const (
	// KindPath expects an existing file path.
	KindPath FieldKind = iota
	// KindOutPath expects a path whose parent directory will be created.
	KindOutPath
	// KindExec expects an executable: a file path or a bare PATH name.
	KindExec
	// KindText is free text.
	KindText
	// KindEnvName expects the name of a set environment variable.
	KindEnvName
	// KindToggle is a boolean toggle.
	KindToggle
)

// FieldSpec declares one form field.
type FieldSpec struct {
	Key         string
	Label       string
	Kind        FieldKind
	Required    bool
	Placeholder string
	Help        string
}

// Field keys used in Form.Values / Form.Toggles.
const (
	keyOutput       = "output"
	keyZipalign     = "zipalign"
	keyApksigner    = "apksigner"
	keyKeytool      = "keytool" // not rendered; prefilled by autodetection
	keyKeystore     = "keystore"
	keyCreateKS     = "create_keystore"
	keyAlias        = "key_alias"
	keyStorePassEnv = "store_pass_env"
	keyKeyPassEnv   = "key_pass_env"
	keySecretEnv    = "secret_env"
	keyPrefix       = "package_prefix"
	keyMultiDex     = "multi_dex"
	keyCompress     = "compress"
	keyRandomPkg    = "random_package"
	keyPseudo       = "pseudo"
	keyReport       = "report"
)

var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// FieldsFor returns the ordered field list shown for a profile.
func FieldsFor(p presets.Profile) []FieldSpec {
	base := []FieldSpec{
		{Key: keyOutput, Label: "Output APK", Kind: KindOutPath, Placeholder: "dist/<name>-protected.apk", Help: "final artifact path (empty = default)"},
		{Key: keyZipalign, Label: "zipalign", Kind: KindExec, Placeholder: "zipalign", Help: "build-tools zipalign binary"},
		{Key: keyApksigner, Label: "apksigner", Kind: KindExec, Placeholder: "apksigner", Help: "build-tools apksigner binary"},
		{Key: keyKeystore, Label: "Keystore", Kind: KindPath, Required: true, Placeholder: "sign/release.keystore", Help: "keystore for V1+V2 signing"},
		{Key: keyCreateKS, Label: "Auto-generate keystore", Kind: KindToggle, Help: "create via keytool when missing"},
		{Key: keyAlias, Label: "Key alias", Kind: KindText, Required: true, Placeholder: "release", Help: "signing key alias"},
		{Key: keyStorePassEnv, Label: "Store password (env name)", Kind: KindEnvName, Required: true, Placeholder: "APK_STORE_PASS", Help: "name of the env var holding the keystore password"},
		{Key: keyKeyPassEnv, Label: "Key password (env name)", Kind: KindEnvName, Placeholder: "APK_KEY_PASS", Help: "empty = same as store password"},
	}
	full := append(base,
		FieldSpec{Key: keySecretEnv, Label: "Encryption secret (env name)", Kind: KindEnvName, Required: true, Placeholder: "APK_PROTECT_SECRET", Help: "name of the env var holding the dex encryption secret"},
		FieldSpec{Key: keyPrefix, Label: "Random package prefix", Kind: KindText, Placeholder: "com.protector", Help: "prefix for randomized package names"},
		FieldSpec{Key: keyMultiDex, Label: "Encrypt all DEX files", Kind: KindToggle, Help: "multi_dex_encrypt"},
		FieldSpec{Key: keyCompress, Label: "Compress before encrypt", Kind: KindToggle, Help: "deflate dex payloads"},
		FieldSpec{Key: keyRandomPkg, Label: "Randomize package name", Kind: KindToggle, Help: "rewrite manifest package"},
		FieldSpec{Key: keyPseudo, Label: "Pseudo hardening markers", Kind: KindToggle, Help: "embed pseudo artifacts"},
	)
	if p == presets.Full {
		return full
	}
	tail := FieldSpec{Key: keyReport, Label: "Report (JSON)", Kind: KindOutPath, Placeholder: "dist/report.json", Help: "machine-readable run report"}
	return append(base, tail)
}

// Form holds the wizard state for one run.
type Form struct {
	Profile  presets.Profile
	InputAPK string
	Values   map[string]string
	Toggles  map[string]bool
}

// NewForm creates a form with profile-appropriate toggle defaults.
func NewForm(profile presets.Profile, input string) *Form {
	f := &Form{
		Profile:  profile,
		InputAPK: input,
		Values:   map[string]string{},
		Toggles:  map[string]bool{},
	}
	if profile == presets.Full {
		f.Toggles[keyMultiDex] = true
		f.Toggles[keyCompress] = true
		f.Toggles[keyRandomPkg] = true
		f.Toggles[keyPseudo] = true
	}
	return f
}

// Fields returns the field list for the form's profile.
func (f *Form) Fields() []FieldSpec { return FieldsFor(f.Profile) }

// DefaultOutputPath returns the output path used when the field is left
// empty: dist/<apk-base>-protected.apk next to the current directory.
func DefaultOutputPath(input string) string {
	base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	return filepath.Join("dist", base+"-protected.apk")
}

// Validate checks every field and returns human-readable errors (empty when
// the form is ready).
func (f *Form) Validate() []string {
	var errs []string
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	for _, spec := range f.Fields() {
		if spec.Kind == KindToggle {
			continue
		}
		value := strings.TrimSpace(f.Values[spec.Key])
		f.Values[spec.Key] = value

		switch spec.Kind {
		case KindText:
			if spec.Required && value == "" {
				add("%s is required", spec.Label)
			}
		case KindEnvName:
			if value == "" {
				if spec.Required {
					add("%s is required (enter the variable name, not the value)", spec.Label)
				}
				continue
			}
			if !envNameRe.MatchString(value) {
				add("%s: %q is not a valid variable name", spec.Label, value)
				continue
			}
			if os.Getenv(value) == "" {
				add("environment variable %s is not set — export it first, the wizard never asks for the secret itself", value)
			}
		case KindExec:
			fallback := "zipalign"
			if spec.Key == keyApksigner {
				fallback = "apksigner"
			}
			if spec.Key == keyKeytool {
				fallback = "keytool"
			}
			name := value
			if name == "" {
				name = fallback
			}
			resolved := expandTilde(name)
			if strings.ContainsRune(resolved, os.PathSeparator) || strings.ContainsRune(resolved, '/') {
				if _, err := os.Stat(resolved); err != nil {
					add("%s: %s does not exist", spec.Label, resolved)
				}
			} else if _, err := exec.LookPath(resolved); err != nil {
				add("%s: %q not found in PATH — install Android build-tools or enter an explicit path", spec.Label, resolved)
			}
		case KindPath:
			if spec.Required && value == "" {
				add("%s is required", spec.Label)
				continue
			}
			if value == "" {
				continue
			}
			resolved := expandTilde(value)
			// The keystore may be auto-generated; anything else must exist.
			if spec.Key == keyKeystore && f.Toggles[keyCreateKS] {
				continue
			}
			if _, err := os.Stat(resolved); err != nil {
				add("%s: %s does not exist", spec.Label, resolved)
			}
		case KindOutPath:
			if value == "" {
				continue
			}
			resolved := expandTilde(value)
			if dir := filepath.Dir(resolved); dir != "." && dir != "/" {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					add("%s: cannot create directory %s", spec.Label, dir)
				}
			}
		}
	}
	return errs
}

// ToConfig materializes the form into an app.Config ready for app.Tool.Run.
// Secrets are resolved from the environment at call time.
func (f *Form) ToConfig() (*app.Config, error) {
	if errs := f.Validate(); len(errs) > 0 {
		return nil, fmt.Errorf("form validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	cfg := &app.Config{
		InputAPK: f.InputAPK,
		Scanning: app.ScanningConfig{Enabled: true},
	}
	presets.Apply(f.Profile, cfg)

	cfg.FinalOutput = expandTilde(f.Values[keyOutput])
	cfg.Zipalign.Path = execOrDefault(f.Values[keyZipalign], "zipalign")
	cfg.Signing.ApksignerPath = execOrDefault(f.Values[keyApksigner], "apksigner")
	cfg.Signing.KeytoolPath = execOrDefault(f.Values[keyKeytool], "keytool")
	cfg.Signing.Keystore = expandTilde(f.Values[keyKeystore])
	cfg.Signing.KeyAlias = f.Values[keyAlias]
	cfg.Signing.CreateKeystore = f.Toggles[keyCreateKS]
	cfg.Signing.StorePass = os.Getenv(f.Values[keyStorePassEnv])
	if env := f.Values[keyKeyPassEnv]; env != "" {
		cfg.Signing.KeyPass = os.Getenv(env)
	} else {
		cfg.Signing.KeyPass = cfg.Signing.StorePass
	}
	cfg.Reporting.JSON = expandTilde(f.Values[keyReport])

	if f.Profile == presets.Full {
		cfg.Protections.EncryptionSecret = os.Getenv(f.Values[keySecretEnv])
		if prefix := strings.TrimSpace(f.Values[keyPrefix]); prefix != "" {
			cfg.Protections.PackagePrefix = prefix
		}
		cfg.Protections.MultiDexEncrypt = f.Toggles[keyMultiDex]
		cfg.Protections.CompressBeforeEncrypt = f.Toggles[keyCompress]
		cfg.Protections.RandomPackage = f.Toggles[keyRandomPkg]
		cfg.Protections.PseudoEncrypt = f.Toggles[keyPseudo]
	}
	return cfg, nil
}

// PreviewJSON renders the config that would be written to disk: identical to
// ToConfig except that secrets appear as ${VAR} references, never as values.
func (f *Form) PreviewJSON() (string, error) {
	cfg, err := f.ToConfig()
	if err != nil {
		return "", err
	}
	if env := f.Values[keyStorePassEnv]; env != "" {
		cfg.Signing.StorePass = "${" + env + "}"
		if f.Values[keyKeyPassEnv] == "" {
			// KeyPass fell back to the store password in ToConfig; keep the
			// file free of the literal value.
			cfg.Signing.KeyPass = "${" + env + "}"
		}
	}
	if env := f.Values[keyKeyPassEnv]; env != "" {
		cfg.Signing.KeyPass = "${" + env + "}"
	}
	if env := f.Values[keySecretEnv]; env != "" {
		cfg.Protections.EncryptionSecret = "${" + env + "}"
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// HeadlessCommand renders the equivalent non-interactive invocation for the
// current form state — shown on the S5 completion screen so a verified wizard
// run can be replayed in CI without reconstructing flags by hand. Only
// non-empty options are emitted, and secrets appear exclusively as env-var
// NAME references (-store-pass-env style flags).
func (f *Form) HeadlessCommand(bin string) string {
	if strings.TrimSpace(bin) == "" {
		bin = "protector"
	}
	args := []string{bin, "run", "-profile", string(f.Profile), "-input", shellQuote(f.InputAPK)}
	add := func(flag, value string) {
		if strings.TrimSpace(value) != "" {
			args = append(args, flag, shellQuote(strings.TrimSpace(value)))
		}
	}

	add("-output", f.Values[keyOutput])
	add("-zipalign", f.Values[keyZipalign])
	add("-apksigner", f.Values[keyApksigner])
	add("-keytool", f.Values[keyKeytool])
	add("-keystore", f.Values[keyKeystore])
	if f.Toggles[keyCreateKS] {
		args = append(args, "-create-keystore")
	}
	add("-key-alias", f.Values[keyAlias])
	add("-store-pass-env", f.Values[keyStorePassEnv])
	add("-key-pass-env", f.Values[keyKeyPassEnv])
	add("-report", f.Values[keyReport])

	if f.Profile == presets.Full {
		add("-protect-secret-env", f.Values[keySecretEnv])
		add("-protect-package-prefix", f.Values[keyPrefix])
		if f.Toggles[keyMultiDex] {
			args = append(args, "-protect-multi-dex")
		}
		if f.Toggles[keyCompress] {
			args = append(args, "-protect-compress")
		}
		if f.Toggles[keyRandomPkg] {
			args = append(args, "-protect-random-package")
		}
		if f.Toggles[keyPseudo] {
			args = append(args, "-protect-pseudo")
		}
	}
	return strings.Join(args, " ")
}

// shellQuote wraps values containing whitespace in single quotes so the
// rendered command stays copy-pasteable.
func shellQuote(v string) string {
	if !strings.ContainsAny(v, " \t'\"") {
		return v
	}
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// CheckStatus classifies a review check line.
type CheckStatus int

const (
	CheckOK CheckStatus = iota
	CheckWarn
	CheckInfo
)

// CheckLine is one entry of the S4 review panel.
type CheckLine struct {
	Label  string
	Status CheckStatus
	Detail string
}

// ReviewChecks builds the pre-run consistency and safety checks.
func (f *Form) ReviewChecks() []CheckLine {
	var checks []CheckLine
	checks = append(checks, CheckLine{
		Label:  "Secrets referenced via environment variables",
		Status: CheckOK,
		Detail: "plain-text secrets never touch the config file",
	})

	if out := expandTilde(f.Values[keyOutput]); out != "" {
		in, _ := filepath.Abs(f.InputAPK)
		outAbs, _ := filepath.Abs(out)
		if in == outAbs {
			checks = append(checks, CheckLine{Label: "Output would overwrite the input APK", Status: CheckWarn, Detail: outAbs})
		} else if insideGitWorkTree(filepath.Dir(outAbs)) {
			checks = append(checks, CheckLine{Label: "Output directory is inside a git work tree", Status: CheckWarn, Detail: filepath.Dir(outAbs) + " — consider dist/ (already gitignored)"})
		}
	}

	keystore := expandTilde(f.Values[keyKeystore])
	if _, err := os.Stat(keystore); err == nil {
		checks = append(checks, CheckLine{Label: "Keystore found", Status: CheckOK, Detail: keystore})
	} else if f.Toggles[keyCreateKS] {
		checks = append(checks, CheckLine{Label: "Keystore will be auto-generated", Status: CheckInfo, Detail: keystore})
	}

	if report := f.Values[keyReport]; report != "" {
		checks = append(checks, CheckLine{Label: "JSON report", Status: CheckInfo, Detail: report})
	}
	return checks
}

// insideGitWorkTree reports whether dir falls inside a git work tree. It
// tolerates a missing git binary (returns false).
func insideGitWorkTree(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// execOrDefault resolves an executable field, falling back to a bare name
// that app.Tool.Run will look up on PATH.
func execOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return expandTilde(strings.TrimSpace(value))
}

// expandTilde expands a leading ~ to the user home directory.
func expandTilde(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
