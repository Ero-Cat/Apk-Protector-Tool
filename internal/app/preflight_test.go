package app

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// TestScanUsesConfiguredKeywords 验证 scanning.keywords 配置真实生效（P0.5：
// 原先读取硬编码表，配置项是死配置）。
func TestScanUsesConfiguredKeywords(t *testing.T) {
	dir := t.TempDir()
	apk := filepath.Join(dir, "app.apk")
	f, err := os.Create(apk)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	entry, err := w.Create("classes.dex")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("this dex mentions custommarker and frida")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// 自定义关键词只含 custommarker：内置的 frida 不应再报。
	results, err := scanAPK(apk, ScanningConfig{Keywords: []string{"custommarker"}, MaxScanSizeMB: 4})
	if err != nil {
		t.Fatalf("scanAPK: %v", err)
	}
	if len(results.AntiEnvironment) != 1 || results.AntiEnvironment[0] != "custommarker" {
		t.Fatalf("anti-environment = %v, want [custommarker]", results.AntiEnvironment)
	}

	// 空配置回退内置表：frida 命中。
	results, err = scanAPK(apk, ScanningConfig{MaxScanSizeMB: 4})
	if err != nil {
		t.Fatalf("scanAPK: %v", err)
	}
	found := false
	for _, kw := range results.AntiEnvironment {
		if kw == "frida" {
			found = true
		}
	}
	if !found {
		t.Fatalf("default keyword frida should hit, got %v", results.AntiEnvironment)
	}
}

// TestPreflightCatchesMissingTools 验证外部工具缺失在管线开始前即报可行动
// 错误（P0.5：原先 zipalign 缺失要到对齐阶段才失败）。
func TestPreflightCatchesMissingTools(t *testing.T) {
	dir := t.TempDir()
	apk := filepath.Join(dir, "app.apk")
	if err := os.WriteFile(apk, []byte("PK\x03\x04stub"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 签名启用但 apksigner 缺失（显式路径不存在）。
	tool := NewTool(&Config{
		InputAPK: apk,
		Signing: SigningConfig{
			Enabled:       true,
			ApksignerPath: filepath.Join(dir, "missing-apksigner"),
		},
	})
	err := tool.preflight(apk)
	if err == nil {
		t.Fatal("preflight should fail for missing apksigner")
	}

	// 产物会被改写（签名启用）且 APK 带 native 库 → 隐式要求 zipalign。
	nativeAPK := filepath.Join(dir, "native.apk")
	zf, err := os.Create(nativeAPK)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	if we, err := zw.Create("lib/arm64-v8a/libdemo.so"); err == nil {
		_, _ = we.Write([]byte("\x7fELF"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zf.Close(); err != nil {
		t.Fatal(err)
	}

	tool2 := NewTool(&Config{
		InputAPK: nativeAPK,
		Signing:  SigningConfig{Enabled: true, Keystore: "whatever.keystore"},
		Zipalign: ZipalignConfig{Path: "/definitely/not/zipalign"},
	})
	if err := tool2.preflight(nativeAPK); err == nil {
		t.Fatal("preflight should fail for missing zipalign on native-lib APK")
	}

	// 纯扫描运行（不改写产物）：隐式 zipalign 不适用，预检通过。
	toolScan := NewTool(&Config{InputAPK: nativeAPK, Scanning: ScanningConfig{Enabled: true}})
	if err := toolScan.preflight(nativeAPK); err != nil {
		t.Fatalf("scan-only preflight should not require zipalign: %v", err)
	}

	// 一切齐备（用假的可执行文件）→ 预检通过。
	fakeZipalign := filepath.Join(dir, "zipalign")
	if err := os.WriteFile(fakeZipalign, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	tool3 := NewTool(&Config{
		InputAPK: nativeAPK,
		Zipalign: ZipalignConfig{Enabled: true, Path: fakeZipalign},
	})
	if err := tool3.preflight(nativeAPK); err != nil {
		t.Fatalf("preflight should pass with valid tools: %v", err)
	}
}
