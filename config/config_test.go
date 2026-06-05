package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadYAML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yml")

	cfgContent := `
input: test.bc
output: out.bc
passes:
  cf_flatten: true
  const_obfuscation: true
obfuscation:
  level: medium
  flatten_ratio: 50
  virtualize_ratio: 30
vmp:
  enable_multi_vm: true
  static_key: "testkeydata"
  vms:
    - name: vm_a
      isa: A
    - name: vm_b
      isa: B
  levels:
    normal: vm_a
    sensitive: vm_b
    critical: vm_b
report:
  path: report.json
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 验证基本字段
	if cfg.Input != "test.bc" {
		t.Errorf("Input = %q, want %q", cfg.Input, "test.bc")
	}
	if cfg.Output != "out.bc" {
		t.Errorf("Output = %q, want %q", cfg.Output, "out.bc")
	}

	// 验证 passes
	if !cfg.Passes.CFFlatten {
		t.Error("Passes.CFFlatten = false, want true")
	}
	if !cfg.Passes.ConstObfuscation {
		t.Error("Passes.ConstObfuscation = false, want true")
	}

	// 验证 obfuscation
	if cfg.Obfuscation.FlattenRatio != 50 {
		t.Errorf("FlattenRatio = %d, want %d", cfg.Obfuscation.FlattenRatio, 50)
	}
	if cfg.Obfuscation.VirtualizeRatio != 30 {
		t.Errorf("VirtualizeRatio = %d, want %d", cfg.Obfuscation.VirtualizeRatio, 30)
	}

	// 验证 VMP
	if !cfg.VMP.EnableMultiVM {
		t.Error("VMP.EnableMultiVM = false, want true")
	}
	if len(cfg.VMP.VMs) != 2 {
		t.Errorf("VMP.VMs len = %d, want %d", len(cfg.VMP.VMs), 2)
	}
	if cfg.VMP.Levels.Normal != "vm_a" {
		t.Errorf("VMP.Levels.Normal = %q, want %q", cfg.VMP.Levels.Normal, "vm_a")
	}

	// 验证 Report
	if cfg.Report.Path != "report.json" {
		t.Errorf("Report.Path = %q, want %q", cfg.Report.Path, "report.json")
	}
}

func TestLoadJSON(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	cfgContent := `{
		"input": "input.bc",
		"output": "output.bc",
		"passes": {
			"const_obfuscation": true
		}
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Input != "input.bc" {
		t.Errorf("Input = %q, want %q", cfg.Input, "input.bc")
	}
	if !cfg.Passes.ConstObfuscation {
		t.Error("Passes.ConstObfuscation = false, want true")
	}
}

func TestLoadDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "minimal.yml")

	// 最小配置
	cfgContent := `
input: test.bc
output: out.bc
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 验证默认值被应用（defaults 函数）
	if cfg.Obfuscation.Level != "medium" {
		t.Errorf("Obfuscation.Level = %q, want default %q", cfg.Obfuscation.Level, "medium")
	}
	// 默认 VMs 应该被创建
	if len(cfg.VMP.VMs) == 0 {
		t.Error("VMP.VMs should have default values")
	}
}

func TestLoadNonExistent(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yml")
	if err == nil {
		t.Error("Load should fail for non-existent file")
	}
}

func TestLoadEmpty(t *testing.T) {
	// 空路径应返回默认配置
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load with empty path failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load with empty path returned nil")
	}
	// 应该有默认 VMs
	if len(cfg.VMP.VMs) == 0 {
		t.Error("VMP.VMs should have default values")
	}
}
