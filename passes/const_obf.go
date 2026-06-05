package passes

import (
	"math/rand"
	"protector-tool/config"
	"protector-tool/llvmwrap"
	"protector-tool/report"
)

// ConstObfPass：加入常量保护钩子。
// 完整方案应遍历并重写字面量，这里先在入口调用解密存根，并用 xor 包裹一次指令结果。
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

		// Wrap first instruction result (if any) with xor identity to hide constants.
		for _, bb := range blocks {
			for _, inst := range bb.Instructions() {
				ops := inst.Operands()
				if len(ops) == 0 {
					continue
				}
				if applied := p.tryWrap(inst); applied {
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

func (p *ConstObfPass) tryWrap(inst llvmwrap.Instruction) bool {
	// Only wrap instructions that produce a value (skip terminators).
	if inst.Opcode() == "ret" || inst.Opcode() == "br" {
		return false
	}
	key := int64(p.Rand.Intn(1<<8) + 1)
	b := llvmwrap.NewBuilderAt(inst)
	k := llvmwrap.ConstInt(key, 64)
	tmp := b.CreateXor(inst.AsValue(), k, "co.x1")
	tmp2 := b.CreateXor(tmp.AsValue(), k, "co.x2")
	inst.ReplaceAllUsesWith(tmp2)
	b.Dispose()
	return true
}
