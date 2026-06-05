package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReportCurrentDirectoryFile(t *testing.T) {
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

	rpt := &Report{Artifacts: map[string]string{}, Hashes: map[string]string{}}
	if err := WriteReport("report.json", rpt); err != nil {
		t.Fatalf("WriteReport() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "report.json")); err != nil {
		t.Fatalf("stat report: %v", err)
	}
}

func TestWriteReportNestedDirectoryFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nested", "report.json")

	rpt := &Report{Artifacts: map[string]string{}, Hashes: map[string]string{}}
	if err := WriteReport(path, rpt); err != nil {
		t.Fatalf("WriteReport() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat nested report: %v", err)
	}
}
