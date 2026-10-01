package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fabricatePlainAPK builds a minimal APK (dex only, no native libs) that can
// run through protections without any Android toolchain.
func fabricatePlainAPK(t *testing.T, dir string) string {
	t.Helper()
	pkg := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "AndroidManifest.xml"),
		[]byte("\x03\x00\x08\x00com.example.keytest"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "classes.dex"),
		[]byte("dex\n035\x00hello keytest payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	apk := filepath.Join(dir, "keytest.apk")
	out, err := os.Create(apk)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	w := zip.NewWriter(out)
	for _, name := range []string{"AndroidManifest.xml", "classes.dex"} {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(mustRead(t, filepath.Join(pkg, name))); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return apk
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestKeyMaterialNotEmbeddedInArtifact 验证 ADR-0001 的核心承诺：
// 默认配置下密钥只落 <final_output>.key（0600），APK 内 metadata.json 不含
// 任何密钥材料，且密文可用外置密钥完整还原。
func TestKeyMaterialNotEmbeddedInArtifact(t *testing.T) {
	dir := t.TempDir()
	apk := fabricatePlainAPK(t, dir)
	plainDex := mustRead(t, filepath.Join(dir, "pkg", "classes.dex"))

	cfg := defaultsForProtectTest(dir, apk)
	cfg.Protections.DexEncrypt = true
	cfg.Protections.PseudoEncrypt = true
	cfg.Protections.EncryptionSecret = "unit-test-secret"

	tool := NewTool(cfg)
	report, err := tool.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	final := report.FinalAPK
	if final == "" {
		t.Fatal("no final artifact")
	}

	// 1. 外置密钥文件存在且权限 0600。
	keyPath := final + ".key"
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("external key file missing: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key file perms = %o, want 0600", info.Mode().Perm())
	}
	var keyMat keyMaterial
	if err := json.Unmarshal(mustRead(t, keyPath), &keyMat); err != nil {
		t.Fatalf("parse key file: %v", err)
	}
	if keyMat.AESKey == "" || keyMat.PseudoKey == "" {
		t.Fatalf("key file incomplete: %+v", keyMat)
	}

	// 2. APK 内不含密钥材料：原文字节、base64、hex 三态扫描。
	apkBytes := mustRead(t, final)
	aesKey, err := base64.StdEncoding.DecodeString(keyMat.AESKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		keyMat.AESKey, keyMat.PseudoKey,
		strings.ToLower(hexEncode(aesKey)),
		"encryption_key",
	} {
		if bytes.Contains(apkBytes, []byte(needle)) {
			t.Fatalf("artifact leaks key material %q", needle)
		}
	}

	// 3. metadata.json（in-APK）只含运行时元数据：nonce 存在、密钥字段缺失。
	meta := readAPKEntry(t, final, "assets/protector/metadata.json")
	var m protectionMetadata
	if err := json.Unmarshal(meta, &m); err != nil {
		t.Fatalf("parse in-apk metadata: %v", err)
	}
	if m.EncryptionKey != "" {
		t.Fatal("metadata.json embeds encryption_key without embed_key=true")
	}
	if m.Pseudo != nil && m.Pseudo.Key != "" {
		t.Fatal("metadata.json embeds pseudo key without embed_key=true")
	}
	if len(m.DexEncrypted) != 1 || m.DexEncrypted[0].Nonce == "" {
		t.Fatalf("runtime metadata incomplete: %+v", m.DexEncrypted)
	}

	// 4. 用外置密钥 + 元数据 nonce 把密文还原成原始 dex 字节。
	cipherBytes := readAPKEntry(t, final, m.DexEncrypted[0].Artifact)
	nonce, err := base64.StdEncoding.DecodeString(m.DexEncrypted[0].Nonce)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := gcm.Open(nil, nonce, cipherBytes, nil)
	if err != nil {
		t.Fatalf("decrypt with external key: %v", err)
	}
	if !bytes.Equal(plain, plainDex) {
		t.Fatal("decrypted dex does not match original")
	}
}

// TestEmbedKeyLegacyMode 锁定 legacy 逃生开关：embed_key=true 时密钥回到
// metadata.json（供旧分析流程），并给出不安全提示。
func TestEmbedKeyLegacyMode(t *testing.T) {
	dir := t.TempDir()
	apk := fabricatePlainAPK(t, dir)

	cfg := defaultsForProtectTest(dir, apk)
	cfg.Protections.DexEncrypt = true
	cfg.Protections.EncryptionSecret = "legacy-secret"
	cfg.Protections.EmbedKey = true

	tool := NewTool(cfg)
	report, err := tool.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	meta := readAPKEntry(t, report.FinalAPK, "assets/protector/metadata.json")
	var m protectionMetadata
	if err := json.Unmarshal(meta, &m); err != nil {
		t.Fatal(err)
	}
	if m.EncryptionKey == "" {
		t.Fatal("legacy embed_key mode must embed the key")
	}
	if _, err := os.Stat(report.FinalAPK + ".key"); err != nil {
		t.Fatalf("legacy mode still writes the external key file: %v", err)
	}
}

func defaultsForProtectTest(dir, apk string) *Config {
	return &Config{
		InputAPK:    apk,
		FinalOutput: filepath.Join(dir, "dist", "keytest-protected.apk"),
		WorkDir:     filepath.Join(dir, "work"),
		Protections: ProtectionConfig{
			Enabled:       true,
			RandomPackage: false,
		},
	}
}

func readAPKEntry(t *testing.T, apkPath, name string) []byte {
	t.Helper()
	r, err := zip.OpenReader(apkPath)
	if err != nil {
		t.Fatalf("open apk: %v", err)
	}
	defer r.Close()
	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	t.Fatalf("entry %q not in apk", name)
	return nil
}

func hexEncode(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, v := range b {
		out = append(out, digits[v>>4], digits[v&0xF])
	}
	return string(out)
}
