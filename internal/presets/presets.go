// Package presets defines reusable protection profiles for the protector
// pipeline. A profile fixes the section switches (scanning, protections,
// zipalign, signing, verification) so that a hardening run needs only an
// input APK plus signing material, instead of a dozen flags.
package presets

import (
	"fmt"

	"github.com/Ero-Cat/Apk-Protector-Tool/internal/app"
)

// Profile is the name of a pipeline preset.
type Profile string

const (
	// Quick runs the security scan, alignment and signing.
	Quick Profile = "quick"
	// Full enables every built-in protection plus alignment, signing and
	// verification.
	Full Profile = "full"
	// SignOnly skips scanning and protections; it aligns and re-signs an
	// existing APK (e.g. after editing a release artifact).
	SignOnly Profile = "sign-only"
)

// Definition describes a profile for menus and help output.
type Definition struct {
	Profile Profile
	Title   string
	Desc    string
	Steps   []string
}

// Definitions returns all built-in profiles in display order.
func Definitions() []Definition {
	return []Definition{
		{
			Profile: Quick,
			Title:   "Quick — scan + align + sign",
			Desc:    "Security scan, zipalign and V1+V2 signing. No built-in protections.",
			Steps:   []string{"scan", "zipalign", "sign"},
		},
		{
			Profile: Full,
			Title:   "Full — all protections + verify",
			Desc:    "Everything the pipeline can do: DEX encryption, package randomization, pseudo markers, alignment, signing and verification.",
			Steps:   []string{"scan", "protect", "zipalign", "sign", "verify"},
		},
		{
			Profile: SignOnly,
			Title:   "Sign-only — align + re-sign",
			Desc:    "Skip scanning and protections; align and re-sign only (e.g. after editing a release APK).",
			Steps:   []string{"zipalign", "sign", "verify"},
		},
	}
}

// Parse converts a profile name into a Profile value.
func Parse(value string) (Profile, error) {
	switch Profile(value) {
	case Quick, Full, SignOnly:
		return Profile(value), nil
	}
	return "", fmt.Errorf("unknown profile %q (want quick, full or sign-only)", value)
}

// Apply overlays the profile's baseline onto cfg. The profile governs the
// section switches and protection toggles; values the profile does not own
// (paths, secrets, package prefix) keep whatever cfg already carries, and
// explicit flag overrides applied afterwards still win.
func Apply(p Profile, cfg *app.Config) {
	switch p {
	case Quick:
		cfg.Scanning.Enabled = true
		setProtections(cfg, false)
		cfg.Zipalign.Enabled = true
		cfg.Signing.Enabled = true
		cfg.Verification.Enabled = false
	case Full:
		cfg.Scanning.Enabled = true
		setProtections(cfg, true)
		cfg.Protections.MultiDexEncrypt = true
		cfg.Protections.CompressBeforeEncrypt = true
		cfg.Protections.RandomPackage = true
		cfg.Protections.PseudoEncrypt = true
		cfg.Zipalign.Enabled = true
		cfg.Signing.Enabled = true
		cfg.Verification.Enabled = true
	case SignOnly:
		cfg.Scanning.Enabled = false
		setProtections(cfg, false)
		cfg.Zipalign.Enabled = true
		cfg.Signing.Enabled = true
		cfg.Verification.Enabled = true
	}
}

// setProtections flips the protection switches without touching secrets or
// the package prefix, so a profile never erases configuration it does not own.
// Disabling also resets the individual toggles so a later re-enable decides
// its own combination instead of inheriting stale ones.
func setProtections(cfg *app.Config, enabled bool) {
	cfg.Protections.Enabled = enabled
	if enabled {
		return
	}
	cfg.Protections.RandomPackage = false
	cfg.Protections.DexEncrypt = false
	cfg.Protections.MultiDexEncrypt = false
	cfg.Protections.PseudoEncrypt = false
	cfg.Protections.CompressBeforeEncrypt = false
}
