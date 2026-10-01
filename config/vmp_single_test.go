package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEnableMultiVMCollapse 验证 enable_multi_vm=false 折叠为单 VM（P0.5：
// 原先该配置从未被读取，多 VM 恒开）。
func TestEnableMultiVMCollapse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "single.yml")
	content := `input: m.bc
vmp:
  enable_multi_vm: false
  vms:
    - name: vm_a
      isa: A
    - name: vm_b
      isa: B
  levels:
    normal: vm_a
    sensitive: vm_b
    critical: vm_b
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.VMP.VMs) != 1 {
		t.Fatalf("single-VM mode should collapse VMs to 1, got %d", len(cfg.VMP.VMs))
	}
	if cfg.VMP.VMs[0].Name != "vm_a" {
		t.Fatalf("collapsed VM = %q, want vm_a", cfg.VMP.VMs[0].Name)
	}
	for _, level := range []string{cfg.VMP.Levels.Normal, cfg.VMP.Levels.Sensitive, cfg.VMP.Levels.Critical} {
		if level != "vm_a" {
			t.Fatalf("all levels should map to vm_a, got %q", level)
		}
	}
}
