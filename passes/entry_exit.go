package passes

import (
	"protector-tool/config"
	"protector-tool/llvmwrap"
	"protector-tool/report"
)

// EntryExitPass：在函数入口/出口插入预留钩子，供运行时做日志或校验。
type EntryExitPass struct {
	Cfg    *config.Config
	Report *report.Report
}

func (p *EntryExitPass) Name() string { return "entry-exit" }

func (p *EntryExitPass) Run(m *llvmwrap.Module) error {
	hookType := llvmwrap.VoidType()
	hookFn := m.EnsureFunction("__goprotect_hook", hookType, nil)

	for _, fn := range m.Functions() {
		if skipFunction(fn.Name(), p.Cfg) {
			continue
		}
		blocks := fn.BasicBlocks()
		if len(blocks) == 0 {
			continue
		}
		first := blocks[0]
		insts := first.Instructions()
		if len(insts) == 0 {
			continue
		}
		// Insert at entry.
		b := llvmwrap.NewBuilderAt(insts[0])
		b.CreateCall(hookFn, nil)
		b.Dispose()

		// Insert before every ret.
		for _, bb := range blocks {
			for _, inst := range bb.Instructions() {
				if inst.Opcode() == "ret" {
					b2 := llvmwrap.NewBuilderAt(inst)
					b2.CreateCall(hookFn, nil)
					b2.Dispose()
				}
			}
		}
		p.Report.MarkInstrumented(fn.Name(), p.Name())
	}
	return nil
}

func skipFunction(name string, cfg *config.Config) bool {
	for _, d := range cfg.Functions.Deny {
		if d == name {
			return true
		}
	}
	if len(cfg.Functions.Allow) == 0 {
		return false
	}
	for _, a := range cfg.Functions.Allow {
		if a == name {
			return false
		}
	}
	return true
}
