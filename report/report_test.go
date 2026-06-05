package report

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCurrentDirectoryFile(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	}()

	rpt := New()
	rpt.AddMessage("ok")
	if err := rpt.Write("report.json"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "report.json")); err != nil {
		t.Fatalf("stat report: %v", err)
	}
}

func TestWriteNestedDirectoryFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nested", "report.json")

	rpt := New()
	rpt.MarkObfuscated("fn", "pass")
	if err := rpt.Write(path); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat nested report: %v", err)
	}
}
