package app

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadZipLimitedRejectsFilesLargerThanLimit(t *testing.T) {
	apk := createZipWithBytes(t, "assets/large.txt", []byte(strings.Repeat("a", 16)))
	reader, err := zip.OpenReader(apk)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer reader.Close()

	_, err = readZipLimited(reader.File[0], 8)
	if err == nil {
		t.Fatal("readZipLimited() error = nil, want file too large")
	}
	if !strings.Contains(err.Error(), "exceeds scan limit") {
		t.Fatalf("readZipLimited() error = %v, want exceeds scan limit", err)
	}
}

func TestScanAPKRecordsSkippedLargeTextFiles(t *testing.T) {
	apk := createZipWithBytes(t, "assets/large.txt", []byte(strings.Repeat("secret=", 200000)))

	result, err := scanAPK(apk, ScanningConfig{
		ApksignerPath: "missing-apksigner",
		MaxScanSizeMB: 1,
	})
	if err != nil {
		t.Fatalf("scanAPK() error = %v", err)
	}
	if len(result.SkippedLargeFiles) != 1 || result.SkippedLargeFiles[0] != "assets/large.txt" {
		t.Fatalf("SkippedLargeFiles = %v, want assets/large.txt", result.SkippedLargeFiles)
	}
}

func createZipWithBytes(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.apk")
	out, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	writer := zip.NewWriter(out)
	w, err := writer.Create(name)
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return path
}
