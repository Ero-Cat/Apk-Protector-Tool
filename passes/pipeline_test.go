package passes

import (
	"math/rand"
	"testing"

	"github.com/Ero-Cat/Apk-Protector-Tool/config"
	"github.com/Ero-Cat/Apk-Protector-Tool/report"
)

func TestBuildPipeline(t *testing.T) {
	cfg := &config.Config{}
	cfg.Passes.ConstObfuscation = true
	cfg.Passes.CFFlatten = true
	cfg.Obfuscation.FlattenRatio = 50
	cfg.Obfuscation.VirtualizeRatio = 30

	rpt := report.New()

	pipeline := BuildPipeline(cfg, rpt)

	if pipeline == nil {
		t.Fatal("BuildPipeline returned nil")
	}

	// 验证 passes 被正确创建
	if len(pipeline.passes) == 0 {
		t.Error("No passes created")
	}
}

func TestBuildPipelineEmpty(t *testing.T) {
	cfg := &config.Config{}
	// 所有 passes 默认 false

	rpt := report.New()

	pipeline := BuildPipeline(cfg, rpt)

	if pipeline == nil {
		t.Fatal("BuildPipeline returned nil")
	}
}

func TestPipelineRunRejectsNilModule(t *testing.T) {
	pipeline := &Pipeline{}

	if err := pipeline.Run(nil); err == nil {
		t.Fatal("Pipeline.Run(nil) error = nil, want error")
	}
}

func TestBuildPipelineWithNilConfigReturnsEmptyPipeline(t *testing.T) {
	pipeline := BuildPipeline(nil, report.New())
	if pipeline == nil {
		t.Fatal("BuildPipeline(nil) returned nil")
	}
	if len(pipeline.passes) != 0 {
		t.Fatalf("BuildPipeline(nil) pass count = %d, want 0", len(pipeline.passes))
	}
}

func TestBuildPipelineWithVMP(t *testing.T) {
	cfg := &config.Config{}
	cfg.Passes.Virtualization = true
	cfg.VMP.EnableMultiVM = true
	cfg.VMP.VMs = []config.VMClass{
		{Name: "vm_a", ISA: "A"},
		{Name: "vm_b", ISA: "B"},
	}
	cfg.VMP.Levels = config.VMLevels{
		Normal:    "vm_a",
		Sensitive: "vm_b",
		Critical:  "vm_b",
	}
	cfg.Obfuscation.VirtualizeRatio = 50

	rpt := report.New()

	pipeline := BuildPipeline(cfg, rpt)

	if pipeline == nil {
		t.Fatal("BuildPipeline returned nil")
	}

	// 验证包含 virtualize pass
	found := false
	for _, p := range pipeline.passes {
		if p.Name() == "virtualize" {
			found = true
			break
		}
	}
	if !found {
		t.Error("virtualize pass not found in pipeline")
	}
}

func TestSkipFunction(t *testing.T) {
	testCases := []struct {
		name     string
		fnName   string
		cfg      *config.Config
		expected bool
	}{
		{
			name:     "regular function no lists",
			fnName:   "myFunction",
			cfg:      &config.Config{},
			expected: false,
		},
		{
			name:   "denied function",
			fnName: "denied_fn",
			cfg: func() *config.Config {
				c := &config.Config{}
				c.Functions.Deny = []string{"denied_fn"}
				return c
			}(),
			expected: true,
		},
		{
			name:   "allowed function",
			fnName: "allowed_fn",
			cfg: func() *config.Config {
				c := &config.Config{}
				c.Functions.Allow = []string{"allowed_fn"}
				return c
			}(),
			expected: false,
		},
		{
			name:   "not in allow list",
			fnName: "other_fn",
			cfg: func() *config.Config {
				c := &config.Config{}
				c.Functions.Allow = []string{"allowed_fn"}
				return c
			}(),
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := skipFunction(tc.fnName, tc.cfg)
			if result != tc.expected {
				t.Errorf("skipFunction(%q) = %v, want %v", tc.fnName, result, tc.expected)
			}
		})
	}
}

func TestCFFlattenPassName(t *testing.T) {
	pass := &CFFlattenPass{
		Cfg:    &config.Config{},
		Rand:   rand.New(rand.NewSource(42)),
		Report: report.New(),
	}

	if pass.Name() != "cf-flatten" {
		t.Errorf("Name() = %q, want %q", pass.Name(), "cf-flatten")
	}
}

func TestVirtualizePassName(t *testing.T) {
	pass := &VirtualizePass{
		Cfg:    &config.Config{},
		Rand:   rand.New(rand.NewSource(42)),
		Report: report.New(),
	}

	if pass.Name() != "virtualize" {
		t.Errorf("Name() = %q, want %q", pass.Name(), "virtualize")
	}
}

func TestShouldRunByRatio(t *testing.T) {
	rnd := rand.New(rand.NewSource(42))
	if shouldRunByRatio(rnd, -1) {
		t.Fatal("shouldRunByRatio(-1) = true, want false")
	}
	if !shouldRunByRatio(rnd, 101) {
		t.Fatal("shouldRunByRatio(101) = false, want true")
	}
}
