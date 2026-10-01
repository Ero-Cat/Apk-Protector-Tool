package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	st := &State{
		LastInput:     "/tmp/app.apk",
		LastOutput:    "dist/app-protected.apk",
		ZipalignPath:  "/opt/bt/zipalign",
		ApksignerPath: "/opt/bt/apksigner",
		KeytoolPath:   "/opt/bt/keytool",
		Keystore:      "sign/release.keystore",
		KeyAlias:      "release",
		ReportPath:    "dist/report.json",
	}
	if err := st.SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	got, err := loadStateFrom(path)
	if err != nil {
		t.Fatalf("loadStateFrom: %v", err)
	}
	if *got != *st {
		t.Fatalf("roundtrip mismatch:\n got  %+v\n want %+v", *got, *st)
	}
}

func TestLoadStateFromMissingFile(t *testing.T) {
	if _, err := loadStateFrom(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("want error for missing file")
	}
}

func TestStateSaveCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "state.json")
	if err := (&State{KeyAlias: "x"}).SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state file not created: %v", err)
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"36.1.0", "35.0.0", 1},
		{"35.0.0", "36.1.0", -1},
		{"36.1.0", "36.1.0", 0},
		{"36.1", "36.1.0", -1},
		{"9.0", "10.0", -1},
		{"", "1.0", -1},
		{"1.0", "", 1},
	}
	for _, tt := range tests {
		if got := compareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestSyncBufferTail(t *testing.T) {
	buf := newSyncBuffer()
	for i := 1; i <= 20; i++ {
		if _, err := buf.Write([]byte(string(rune('a'+i-1)) + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	tail := buf.tail(3)
	if tail != "r\ns\nt" {
		t.Fatalf("tail(3) = %q", tail)
	}
}
