package app

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractZipFileRejectsTraversalEntries(t *testing.T) {
	tests := []struct {
		name  string
		entry string
	}{
		{name: "parent directory", entry: "../evil.txt"},
		{name: "nested parent directory", entry: "res/../../evil.txt"},
		{name: "absolute path", entry: "/tmp/evil.txt"},
		{name: "windows separator traversal", entry: `res\..\evil.txt`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apk := createTestZip(t, map[string]string{tt.entry: "evil"})
			dest := t.TempDir()

			_, err := unzipArchive(apk, dest)
			if err == nil {
				t.Fatalf("unzipArchive(%q) error = nil, want invalid zip entry", tt.entry)
			}
			if !strings.Contains(err.Error(), "invalid zip entry") {
				t.Fatalf("unzipArchive(%q) error = %v, want invalid zip entry", tt.entry, err)
			}
		})
	}
}

func TestExtractZipFileAllowsSiblingDotPrefix(t *testing.T) {
	apk := createTestZip(t, map[string]string{"..not-parent/file.txt": "ok"})
	dest := t.TempDir()

	if _, err := unzipArchive(apk, dest); err != nil {
		t.Fatalf("unzipArchive() error = %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dest, "..not-parent", "file.txt")); err != nil || string(got) != "ok" {
		t.Fatalf("extracted file = %q, %v; want ok", got, err)
	}
}

func TestZipDirectoryPreservesStoredNativeLibMethod(t *testing.T) {
	src := t.TempDir()
	libPath := filepath.Join(src, "lib", "arm64-v8a", "libdemo.so")
	if err := os.MkdirAll(filepath.Dir(libPath), 0o755); err != nil {
		t.Fatalf("create lib dir: %v", err)
	}
	if err := os.WriteFile(libPath, []byte("so"), 0o644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	out := filepath.Join(t.TempDir(), "out.apk")

	if err := zipDirectory(src, out, nil); err != nil {
		t.Fatalf("zipDirectory() error = %v", err)
	}

	reader, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name == "lib/arm64-v8a/libdemo.so" {
			if file.Method != zip.Store {
				t.Fatalf("native lib method = %d, want Store", file.Method)
			}
			return
		}
	}
	t.Fatal("native lib missing from zip")
}

func createTestZip(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.apk")
	out, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	writer := zip.NewWriter(out)
	for name, content := range files {
		w, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create entry: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return path
}
