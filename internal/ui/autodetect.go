package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// BuildTools bundles the detected Android build-tools binaries.
type BuildTools struct {
	Zipalign  string
	Apksigner string
	Keytool   string
}

// DetectBuildTools locates the newest Android build-tools directory and the
// three binaries the pipeline needs. Missing pieces fall back to PATH lookup
// and may end up empty when nothing is found.
func DetectBuildTools() BuildTools {
	var roots []string
	for _, env := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if v := os.Getenv(env); v != "" {
			roots = append(roots, v)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots,
			filepath.Join(home, "Library", "Android", "sdk"),
			filepath.Join(home, "Android", "Sdk"),
		)
	}

	bestDir := ""
	bestVersion := ""
	for _, root := range roots {
		btDir := filepath.Join(root, "build-tools")
		entries, err := os.ReadDir(btDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join(btDir, entry.Name(), "zipalign")); err != nil {
				continue
			}
			if compareVersions(entry.Name(), bestVersion) > 0 {
				bestDir = filepath.Join(btDir, entry.Name())
				bestVersion = entry.Name()
			}
		}
	}

	bt := BuildTools{}
	if bestDir != "" {
		bt.Zipalign = filepath.Join(bestDir, "zipalign")
		if p := filepath.Join(bestDir, "apksigner"); fileThere(p) {
			bt.Apksigner = p
		}
		if p := filepath.Join(bestDir, "keytool"); fileThere(p) {
			bt.Keytool = p
		}
	}
	if bt.Zipalign == "" {
		bt.Zipalign, _ = exec.LookPath("zipalign")
	}
	if bt.Apksigner == "" {
		bt.Apksigner, _ = exec.LookPath("apksigner")
	}
	if bt.Keytool == "" {
		bt.Keytool, _ = exec.LookPath("keytool")
	}
	return bt
}

func fileThere(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// compareVersions orders dotted numeric version strings ("36.1.0" > "35.0.0").
// Non-numeric segments compare lexically.
func compareVersions(a, b string) int {
	if a == "" {
		return -1
	}
	if b == "" {
		return 1
	}
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		av, bv := "", ""
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		an, aerr := strconv.Atoi(av)
		bn, berr := strconv.Atoi(bv)
		switch {
		case aerr != nil || berr != nil:
			if av != bv {
				return strings.Compare(av, bv)
			}
		case an != bn:
			if an < bn {
				return -1
			}
			return 1
		}
	}
	return 0
}

// looksLikeAPK reports whether path starts with the zip magic bytes shared
// by APK files.
func looksLikeAPK(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	magic := make([]byte, 2)
	if _, err := f.Read(magic); err != nil {
		return false
	}
	return string(magic) == "PK"
}

// isTerminal reports whether f is attached to a character device.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
