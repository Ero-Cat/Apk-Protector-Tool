package passes

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/Ero-Cat/Apk-Protector-Tool/config"
	"github.com/Ero-Cat/Apk-Protector-Tool/llvmwrap"
	"github.com/Ero-Cat/Apk-Protector-Tool/report"
)

// Pass 定义：对模块进行一次 IR 变换。
type Pass interface {
	Name() string
	Run(m *llvmwrap.Module) error
}

// Pipeline 依次执行一组 Pass。
type Pipeline struct {
	passes []Pass
}

func (p *Pipeline) Run(m *llvmwrap.Module) error {
	if m == nil {
		return fmt.Errorf("module is nil")
	}
	for _, pass := range p.passes {
		if err := pass.Run(m); err != nil {
			return fmt.Errorf("%s: %w", pass.Name(), err)
		}
		// 每个 Pass 之后跑一次 verifier：损坏的模块在源头点名，
		// 而不是静默写出非法位码（P4.2 的逐 pass 化）。
		if err := m.Verify(); err != nil {
			if path := os.Getenv("GOPROTECT_DUMP_IR"); path != "" {
				_ = os.WriteFile(path, []byte(m.String()), 0o644)
			}
			return fmt.Errorf("module invalid after pass %s: %w", pass.Name(), err)
		}
	}
	return nil
}

func shouldRunByRatio(rnd *rand.Rand, ratio int) bool {
	if ratio <= 0 {
		return false
	}
	if ratio >= 100 {
		return true
	}
	return rnd.Intn(100) < ratio
}

// BuildPipeline 按配置构建可控的 Pass 序列。
func BuildPipeline(cfg *config.Config, rpt *report.Report) *Pipeline {
	if cfg == nil {
		return &Pipeline{}
	}
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

	passes := []Pass{}
	if cfg.Passes.EntryExit {
		passes = append(passes, &EntryExitPass{Cfg: cfg, Report: rpt})
	}
	if cfg.Passes.ConstSplit {
		passes = append(passes, &ConstSplitPass{Cfg: cfg, Rand: rnd, Report: rpt})
	}
	if cfg.Passes.InstrSubstitute {
		passes = append(passes, &InstrSubPass{Cfg: cfg, Rand: rnd, Report: rpt})
	}
	if cfg.Passes.CFFlatten {
		passes = append(passes, &CFFlattenPass{Cfg: cfg, Rand: rnd, Report: rpt})
	}
	if cfg.Passes.ConstObfuscation {
		passes = append(passes, &ConstObfPass{Cfg: cfg, Rand: rnd, Report: rpt})
	}
	if cfg.Passes.Virtualization {
		passes = append(passes, &VirtualizePass{Cfg: cfg, Rand: rnd, Report: rpt})
	}
	if cfg.Passes.SecurityHooks {
		passes = append(passes, &SecurityHooksPass{Cfg: cfg, Report: rpt})
	}

	return &Pipeline{passes: passes}
}
