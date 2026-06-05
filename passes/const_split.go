package passes

import (
	"math/rand"
	"protector-tool/config"
	"protector-tool/llvmwrap"
	"protector-tool/report"
)

// ConstSplitPass：把整型立即数拆成多步加法，弱化静态特征。
type ConstSplitPass struct {
	Cfg    *config.Config
	Rand   *rand.Rand
	Report *report.Report
}

func (p *ConstSplitPass) Name() string { return "const-split" }

func (p *ConstSplitPass) Run(m *llvmwrap.Module) error {
	for _, fn := range m.Functions() {
		if skipFunction(fn.Name(), p.Cfg) {
			continue
		}
		changed := false
		for _, bb := range fn.BasicBlocks() {
			for _, inst := range bb.Instructions() {
				if inst.Opcode() != "add" {
					continue
				}
				ops := inst.Operands()
				if len(ops) != 2 {
					continue
				}
				// Find a constant operand.
				var other llvmwrap.Value
				var constVal uint64
				var ok bool
				if yes, v := ops[0].IsConstInt(); yes {
					constVal = v
					other = ops[1]
					ok = true
				} else if yes, v := ops[1].IsConstInt(); yes {
					constVal = v
					other = ops[0]
					ok = true
				}
				if !ok || constVal == 0 {
					continue
				}
				bitWidth := 64
				if w, ok := other.Type().IntWidth(); ok {
					bitWidth = w
				}
				half := constVal / 2
				rest := constVal - half
				b := llvmwrap.NewBuilderAt(inst)
				tmp1 := b.CreateAdd(other, llvmwrap.ConstInt(int64(half), uint(bitWidth)), "cs1")
				tmp2 := b.CreateAdd(tmp1.AsValue(), llvmwrap.ConstInt(int64(rest), uint(bitWidth)), "cs2")
				inst.ReplaceAllUsesWith(tmp2)
				b.Dispose()
				changed = true
			}
		}
		if changed {
			p.Report.MarkObfuscated(fn.Name(), p.Name())
		}
	}
	return nil
}
