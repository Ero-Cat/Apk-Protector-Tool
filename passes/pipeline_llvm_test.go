//go:build llvm

// LLVM-tagged integration tests: real parse → pass → verify → structure
// assertions on fixed .ll fixtures (ROADMAP P4.2). Semantic equivalence is
// checked by running main under lli before and after the pipeline; runtime
// symbols (VM entries, hooks) resolve via --extra-object with the object
// path from GOPROTECT_TEST_RUNTIME_OBJ.
package passes

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ero-Cat/Apk-Protector-Tool/config"
	"github.com/Ero-Cat/Apk-Protector-Tool/llvmwrap"
	"github.com/Ero-Cat/Apk-Protector-Tool/report"
)

func loadFixture(t *testing.T, name string) *llvmwrap.Module {
	t.Helper()
	m, err := llvmwrap.ParseBitcode(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	t.Cleanup(m.Dispose)
	return m
}

// singlePassCfg starts from the default config, disables every pass and
// re-enables only what the callback turns on — defaults() would otherwise
// auto-enable a whole bundle.
func singlePassCfg(t *testing.T, enable func(*config.Config)) *config.Config {
	t.Helper()
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load default config: %v", err)
	}
	cfg.Passes.EntryExit = false
	cfg.Passes.ConstSplit = false
	cfg.Passes.InstrSubstitute = false
	cfg.Passes.CFFlatten = false
	cfg.Passes.ConstObfuscation = false
	cfg.Passes.Virtualization = false
	cfg.Passes.SecurityHooks = false
	if enable != nil {
		enable(cfg)
	}
	return cfg
}

// fullPipelineCfg enables every pass at maximum strength.
func fullPipelineCfg(t *testing.T) *config.Config {
	t.Helper()
	return singlePassCfg(t, func(cfg *config.Config) {
		cfg.Passes.EntryExit = true
		cfg.Passes.ConstSplit = true
		cfg.Passes.InstrSubstitute = true
		cfg.Passes.CFFlatten = true
		cfg.Passes.ConstObfuscation = true
		cfg.Passes.Virtualization = true
		cfg.Passes.SecurityHooks = true
		cfg.Obfuscation.SubstituteIntensity = 10
		cfg.Obfuscation.FlattenRatio = 100
		cfg.Obfuscation.VirtualizeRatio = 100
	})
}

func runPipeline(t *testing.T, fixture string, cfg *config.Config) *llvmwrap.Module {
	t.Helper()
	m := loadFixture(t, fixture)
	p := BuildPipeline(cfg, report.New())
	if err := p.Run(m); err != nil {
		t.Fatalf("pipeline on %s: %v", fixture, err)
	}
	if err := m.Verify(); err != nil {
		t.Fatalf("verify after pipeline on %s: %v", fixture, err)
	}
	return m
}

func lliAvailable() bool {
	_, err := exec.LookPath("lli")
	return err == nil
}

func runtimeObjPath() string { return os.Getenv("GOPROTECT_TEST_RUNTIME_OBJ") }

// runLli executes the module's main under lli and returns its exit code.
func runLli(t *testing.T, m *llvmwrap.Module, extraObject string) int {
	t.Helper()
	bc := filepath.Join(t.TempDir(), "mod.bc")
	if err := m.WriteBitcode(bc); err != nil {
		t.Fatalf("write bitcode: %v", err)
	}
	args := []string{}
	if extraObject != "" {
		args = append(args, "--extra-object="+extraObject)
	}
	args = append(args, bc)
	err := exec.Command("lli", args...).Run()
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	t.Fatalf("run lli: %v", err)
	return -1
}

// lliOutput runs main under lli capturing stdout (string fixtures).
func lliOutput(t *testing.T, m *llvmwrap.Module, extraObject string) string {
	t.Helper()
	bc := filepath.Join(t.TempDir(), "mod.bc")
	if err := m.WriteBitcode(bc); err != nil {
		t.Fatalf("write bitcode: %v", err)
	}
	args := []string{}
	if extraObject != "" {
		args = append(args, "--extra-object="+extraObject)
	}
	args = append(args, bc)
	out, err := exec.Command("lli", args...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("lli exited %d: %s", ee.ExitCode(), ee.Stderr)
		}
		t.Fatalf("run lli: %v", err)
	}
	return string(out)
}

func TestTextualAndBitcodeInputsEquivalent(t *testing.T) {
	ll := loadFixture(t, "arith.ll")
	bcPath := filepath.Join(t.TempDir(), "arith.bc")
	if err := ll.WriteBitcode(bcPath); err != nil {
		t.Fatalf("write bitcode: %v", err)
	}
	bc, err := llvmwrap.ParseBitcode(bcPath)
	if err != nil {
		t.Fatalf("re-parse bitcode: %v", err)
	}
	t.Cleanup(bc.Dispose)
	if stripModuleID(ll.String()) != stripModuleID(bc.String()) {
		t.Fatal("same module via .ll and .bc renders different IR")
	}
}

// stripModuleID drops the leading ModuleID comment, which carries the source
// file path and legitimately differs between the two parse paths.
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

func TestParseRejectsGarbageInput(t *testing.T) {
	p := filepath.Join(t.TempDir(), "garbage.ll")
	if err := os.WriteFile(p, []byte("this is not llvm ir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := llvmwrap.ParseBitcode(p); err == nil {
		t.Fatal("expected parse error for garbage .ll input")
	}
}

func TestFullPipelinePreservesSemantics(t *testing.T) {
	if !lliAvailable() {
		t.Skip("lli not in PATH")
	}
	base := loadFixture(t, "arith.ll")
	if got := runLli(t, base, ""); got != 13 {
		t.Fatalf("baseline lli exit = %d, want 13", got)
	}
	m := runPipeline(t, "arith.ll", fullPipelineCfg(t))
	obj := runtimeObjPath()
	if obj == "" {
		t.Log("GOPROTECT_TEST_RUNTIME_OBJ not set; skipping post-pipeline lli run")
		return
	}
	if got := runLli(t, m, obj); got != 13 {
		t.Fatalf("post-pipeline lli exit = %d, want 13", got)
	}
}

func TestCFFlattenRewritesControlFlow(t *testing.T) {
	cfg := singlePassCfg(t, func(cfg *config.Config) {
		cfg.Passes.CFFlatten = true
		cfg.Obfuscation.FlattenRatio = 100
	})
	m := runPipeline(t, "branchy.ll", cfg)
	ir := m.String()
	if !strings.Contains(ir, "switch i32") {
		t.Fatal("flattened module has no dispatcher switch")
	}
	if lliAvailable() {
		if got := runLli(t, m, ""); got != 20 {
			t.Fatalf("post-flatten lli exit = %d, want 20", got)
		}
	}
}

func TestDumpCFGDOTStructure(t *testing.T) {
	m := loadFixture(t, "arith.ll")
	dot := DumpCFGDOT(m)
	for _, want := range []string{"digraph cfg", `"add"`, "->"} {
		if !strings.Contains(dot, want) {
			t.Fatalf("DOT output missing %q:\n%s", want, dot)
		}
	}
}

func TestVirtualizeVoidOnlyBaseline(t *testing.T) {
	// Current ABI virtualizes only void functions without argument
	// references; arith.ll has none, so nothing may change structurally.
	// Flipped to assert real virtualization by P2.3.
	cfg := singlePassCfg(t, func(cfg *config.Config) {
		cfg.Passes.Virtualization = true
		cfg.Obfuscation.VirtualizeRatio = 100
	})
	m := runPipeline(t, "arith.ll", cfg)
	ir := m.String()
	if !strings.Contains(ir, "add nsw i32") {
		t.Fatal("add function body vanished before non-void virtualization landed")
	}
}
