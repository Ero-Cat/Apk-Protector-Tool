//go:build llvm

// Native smoke tests for the llvmwrap facade (ROADMAP P4.2.4): parse a real
// textual module, verify it, round-trip through bitcode and render IR.
package llvmwrap

import (
	"path/filepath"
	"strings"
	"testing"
)

func loadMini(t *testing.T) *Module {
	t.Helper()
	m, err := ParseBitcode(filepath.Join("testdata", "mini.ll"))
	if err != nil {
		t.Fatalf("parse mini.ll: %v", err)
	}
	t.Cleanup(m.Dispose)
	return m
}

func TestNativeParseVerifyRoundtrip(t *testing.T) {
	m := loadMini(t)
	if err := m.Verify(); err != nil {
		t.Fatalf("verify: %v", err)
	}
	fns := m.Functions()
	if len(fns) != 1 || fns[0].Name() != "answer" {
		t.Fatalf("expected single function @answer, got %d", len(fns))
	}

	bc := filepath.Join(t.TempDir(), "mini.bc")
	if err := m.WriteBitcode(bc); err != nil {
		t.Fatalf("write bitcode: %v", err)
	}
	m2, err := ParseBitcode(bc)
	if err != nil {
		t.Fatalf("re-parse bitcode: %v", err)
	}
	t.Cleanup(m2.Dispose)
	if stripModuleID(m.String()) != stripModuleID(m2.String()) {
		t.Fatal("bitcode round-trip renders different IR")
	}
}

// stripModuleID drops the leading ModuleID comment, which carries the source
// file path and legitimately differs between the .ll and .bc parse paths.
func stripModuleID(ir string) string {
	lines := strings.Split(ir, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.HasPrefix(l, "; ModuleID = ") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func TestNativeStringContainsFunction(t *testing.T) {
	m := loadMini(t)
	ir := m.String()
	if !strings.Contains(ir, "@answer") || !strings.Contains(ir, "ret i32 42") {
		t.Fatalf("rendered IR missing fixture content:\n%s", ir)
	}
}
