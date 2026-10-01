package passes

import (
	"github.com/Ero-Cat/Apk-Protector-Tool/config"
	"github.com/Ero-Cat/Apk-Protector-Tool/llvmwrap"
	"github.com/Ero-Cat/Apk-Protector-Tool/report"
)

// SecurityHooksPass：在关键函数入口插入完整性与反调试钩子，实际实现由运行时提供。
type SecurityHooksPass struct {
	Cfg    *config.Config
	Report *report.Report
}

func (p *SecurityHooksPass) Name() string { return "security-hooks" }

func (p *SecurityHooksPass) Run(m *llvmwrap.Module) error {
	intFn := m.EnsureFunction("__goprotect_check_integrity", llvmwrap.VoidType(), []llvmwrap.ValueType{llvmwrap.IntType(32)})
	dbgFn := m.EnsureFunction("__goprotect_anti_debug", llvmwrap.VoidType(), nil)

	id := uint64(1)
	for _, fn := range m.Functions() {
		if skipFunction(fn.Name(), p.Cfg) {
			continue
		}
		blocks := fn.BasicBlocks()
		if len(blocks) == 0 {
			continue
		}
		insts := blocks[0].Instructions()
		if len(insts) == 0 {
			continue
		}
		b := llvmwrap.NewBuilderAt(insts[0])
		b.CreateCall(intFn, []llvmwrap.Value{llvmwrap.ConstInt(int64(id), 32)})
		b.CreateCall(dbgFn, nil)
		b.Dispose()
		id++
		p.Report.MarkInstrumented(fn.Name(), p.Name())
	}
	return nil
}
