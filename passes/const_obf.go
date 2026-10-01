package passes

import (
	"math/rand"

	"github.com/Ero-Cat/Apk-Protector-Tool/config"
	"github.com/Ero-Cat/Apk-Protector-Tool/llvmwrap"
	"github.com/Ero-Cat/Apk-Protector-Tool/report"
)

// ConstObfPass：加入常量保护钩子。
// 完整方案应遍历并重写字面量（ROADMAP P2.2）；当前在入口调用解密存根，
// 并对首个整型操作数做 xor 恒等包裹以隐藏直接常量引用。
type ConstObfPass struct {
	Cfg    *config.Config
	Rand   *rand.Rand
	Report *report.Report
}

func (p *ConstObfPass) Name() string { return "const-obf" }

func (p *ConstObfPass) Run(m *llvmwrap.Module) error {
	decFn := m.EnsureFunction("__goprotect_decrypt_strings", llvmwrap.VoidType(), nil)

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
		// One-time decrypt stub at entry.
		b := llvmwrap.NewBuilderAt(insts[0])
		b.CreateCall(decFn, nil)
		b.Dispose()

		// Wrap the first integer operand we can find (identity xor chain).
		for _, bb := range blocks {
			for _, inst := range bb.Instructions() {
				ops := inst.Operands()
				if len(ops) == 0 {
					continue
				}
				if wrapIntegerOperand(p.Rand, inst) {
					p.Report.MarkObfuscated(fn.Name(), p.Name())
					goto nextFn
				}
			}
		}
	nextFn:
		continue
	}
	return nil
}
