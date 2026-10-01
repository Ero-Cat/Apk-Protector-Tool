package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRunner 模拟外部二进制（P4.1）：支持"工具缺失/执行失败/成功"三类
// 路径，并仿真 zipalign / apksigner / keytool 的产物落盘行为，使全链路
// 编排无需任何 Android 工具即可断言。
type fakeRunner struct {
	missing map[string]bool // LookPath 报 not found
	failBin string          // 该二进制执行失败
	ran     []string
}

func (f *fakeRunner) LookPath(bin string) (string, error) {
	if f.missing[bin] {
		return "", exec.ErrNotFound
	}
	return bin, nil
}

func (f *fakeRunner) Output(bin string, args []string) ([]byte, error) {
	f.ran = append(f.ran, bin+":verify-scan")
	if bin == f.failBin {
		return nil, fmt.Errorf("exit status 1 (simulated)")
	}
	// scanner 验签输出：带 V2 true 的摘要即可。
	return []byte("Verifications\nSigner: CN=Test\nAPK Signature Scheme v2: true\n"), nil
}

func (f *fakeRunner) Run(ctx context.Context, bin string, args []string, env map[string]string,
	timeout time.Duration, stdout, stderr io.Writer) error {
	f.ran = append(f.ran, bin)
	if bin == f.failBin {
		return fmt.Errorf("exit status 1 (simulated)")
	}
	switch {
	case strings.HasSuffix(bin, "zipalign") && len(args) >= 2:
		// zipalign -p -f <align> <in> <out>
		return copyFileForTest(args[len(args)-2], args[len(args)-1])
	case strings.HasSuffix(bin, "keytool"):
		// -genkeypair ... -keystore <path>
		for i, a := range args {
			if a == "-keystore" && i+1 < len(args) {
				return os.WriteFile(args[i+1], []byte("fake-keystore"), 0o600)
			}
		}
		return nil
	case strings.HasSuffix(bin, "apksigner") && len(args) > 0 && args[0] == "sign":
		// sign ... --out <output> <input>
		out := ""
		for i, a := range args {
			if a == "--out" && i+1 < len(args) {
				out = args[i+1]
			}
		}
		if out == "" {
			return nil
		}
		return copyFileForTest(args[len(args)-1], out)
	default:
		return nil // verify 等只读命令
	}
}

func copyFileForTest(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func orchestrationConfig(t *testing.T, dir string) *Config {
	t.Helper()
	apk := fabricatePlainAPK(t, dir)
	ks := filepath.Join(dir, "test.keystore")
	if err := os.WriteFile(ks, []byte("fake-keystore"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{
		InputAPK:    apk,
		FinalOutput: filepath.Join(dir, "dist", "orch-protected.apk"),
		WorkDir:     filepath.Join(dir, "work"),
		Scanning:    ScanningConfig{Enabled: true, MaxScanSizeMB: 4},
		Protections: ProtectionConfig{
			Enabled:    true,
			DexEncrypt: true,
		},
		Zipalign: ZipalignConfig{Enabled: true, Path: "fake-zipalign"},
		Signing: SigningConfig{
			Enabled:       true,
			Keystore:      ks,
			KeyAlias:      "test",
			StorePass:     "store",
			KeyPass:       "key",
			ApksignerPath: "fake-apksigner",
			KeytoolPath:   "fake-keytool",
		},
		Verification: VerificationConfig{Enabled: true},
	}
	return cfg
}

func stepStatuses(r *Report) map[string]string {
	out := map[string]string{}
	for _, s := range r.Steps {
		out[s.Name] = s.Status
	}
	return out
}

// TestOrchestrationMissingToolFailsFast：preflight 在任何阶段之前拦下缺失的
// 外部工具，错误信息可行动（点名工具与安装建议）。
func TestOrchestrationMissingToolFailsFast(t *testing.T) {
	dir := t.TempDir()
	cfg := orchestrationConfig(t, dir)

	tool := NewTool(cfg)
	tool.Runner = &fakeRunner{missing: map[string]bool{"fake-apksigner": true}}

	report, err := tool.Run(context.Background())
	if err == nil {
		t.Fatal("expected preflight failure for missing apksigner")
	}
	if !strings.Contains(err.Error(), "signing") || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error not actionable: %v", err)
	}
	// 预检在任何阶段之前失败：报告尚未生成（nil）。
	if report != nil && len(report.Steps) != 0 {
		t.Fatalf("no stage may run before preflight, got %v", stepStatuses(report))
	}
}

// TestOrchestrationCommandFailurePropagates：中段工具失败保留前序已完成
// 阶段，失败步骤状态与错误都指向具体工具。
func TestOrchestrationCommandFailurePropagates(t *testing.T) {
	dir := t.TempDir()
	cfg := orchestrationConfig(t, dir)

	tool := NewTool(cfg)
	tool.Runner = &fakeRunner{failBin: "fake-zipalign"}

	report, err := tool.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "zipalign") {
		t.Fatalf("expected zipalign failure, got %v", err)
	}
	steps := stepStatuses(report)
	if steps["scan"] != "completed" || steps["protections"] != "completed" {
		t.Fatalf("earlier stages must stay completed: %v", steps)
	}
	if steps["zipalign"] != "failed" {
		t.Fatalf("zipalign step must be failed: %v", steps)
	}
	if steps["sign"] != "" {
		t.Fatalf("signing must not run after align failure: %v", steps)
	}
}

// TestOrchestrationFullSuccess：假 runner 全绿时 scan→protect→align→sign→
// verify→finalize 全链路完成，产物/密钥/报告工件齐备。
func TestOrchestrationFullSuccess(t *testing.T) {
	dir := t.TempDir()
	cfg := orchestrationConfig(t, dir)

	tool := NewTool(cfg)
	runner := &fakeRunner{}
	tool.Runner = runner

	report, err := tool.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	steps := stepStatuses(report)
	for _, stage := range []string{"scan", "protections", "zipalign", "sign", "verify"} {
		if steps[stage] != "completed" {
			t.Fatalf("stage %s = %q, want completed (all: %v)", stage, steps[stage], steps)
		}
	}

	if report.FinalAPK == "" {
		t.Fatal("no final artifact")
	}
	if _, err := os.Stat(report.FinalAPK); err != nil {
		t.Fatalf("final artifact missing: %v", err)
	}
	if _, err := os.Stat(report.FinalAPK + ".key"); err != nil {
		t.Fatalf("external key file missing: %v", err)
	}
	if report.Artifacts["signed_apk"] == "" || report.Artifacts["zipaligned_apk"] == "" {
		t.Fatalf("stage artifacts missing: %v", report.Artifacts)
	}

	// keystore 已存在：keytool 不应被调用。
	for _, invoked := range runner.ran {
		if strings.HasSuffix(invoked, "keytool") {
			t.Fatalf("keytool must be skipped when keystore exists: %v", runner.ran)
		}
	}
}

// TestOrchestrationKeystoreGeneration：keystore 缺失 + CreateKeystore 时
// 经 runner 调 keytool 生成。
func TestOrchestrationKeystoreGeneration(t *testing.T) {
	dir := t.TempDir()
	cfg := orchestrationConfig(t, dir)
	if err := os.Remove(cfg.Signing.Keystore); err != nil {
		t.Fatal(err)
	}
	cfg.Signing.CreateKeystore = true

	tool := NewTool(cfg)
	runner := &fakeRunner{}
	tool.Runner = runner

	if _, err := tool.Run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}
	sawKeytool := false
	for _, invoked := range runner.ran {
		if strings.HasSuffix(invoked, "keytool") {
			sawKeytool = true
		}
	}
	if !sawKeytool {
		t.Fatalf("keytool must be invoked to generate the keystore: %v", runner.ran)
	}
}
