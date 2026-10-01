package passes

import (
	"math/rand"

	"github.com/Ero-Cat/Apk-Protector-Tool/config"
	"github.com/Ero-Cat/Apk-Protector-Tool/llvmwrap"
	"github.com/Ero-Cat/Apk-Protector-Tool/report"
)

// InstrSubPass：用可逆的 xor 恒等链包裹指令的整型操作数，达到轻量替换效果。
// 与 ConstObfPass 共用 wrapOperand（密钥经栈槽中转，规避 IRBuilder 常数折叠）。
type InstrSubPass struct {
	Cfg    *config.Config
	Rand   *rand.Rand
	Report *report.Report
}

func (p *InstrSubPass) Name() string { return "instr-sub" }

func (p *InstrSubPass) Run(m *llvmwrap.Module) error {
	intensity := p.Cfg.Obfuscation.SubstituteIntensity
	if intensity <= 0 {
		return nil
	}
	for _, fn := range m.Functions() {
		if skipFunction(fn.Name(), p.Cfg) {
			continue
		}
		blocks := fn.BasicBlocks()
		if len(blocks) == 0 {
			continue
		}
		entryInsts := blocks[0].Instructions()
		if len(entryInsts) == 0 {
			continue
		}
		ks := newKeySlots(entryInsts[0])

		changed := false
		for _, bb := range blocks {
			for _, inst := range bb.Instructions() {
				// 终结器与 phi 不做操作数包裹：switch case 值必须是常量，
				// phi 入参的支配关系在插入重写指令后无法保证（见 const_obf.go）。
				if noWrapOpcodes[inst.Opcode()] {
					continue
				}
				// Apply with probability derived from intensity.
				if p.Rand.Intn(10) >= intensity {
					continue
				}
				if wrapFirstIntegerOperand(p.Rand, ks, inst) {
					changed = true
				}
			}
		}
		if changed {
			p.Report.MarkObfuscated(fn.Name(), p.Name())
		}
	}
	return nil
}

// wrapFirstIntegerOperand wraps the instruction's first integer operand with
// an identity chain (see wrapOperand); returns false when there is none.
func wrapFirstIntegerOperand(r *rand.Rand, ks *keySlots, inst llvmwrap.Instruction) bool {
	for idx, op := range inst.Operands() {
		width, ok := op.Type().IntWidth()
		if !ok {
			continue
		}
		wrapOperand(r, ks, inst, idx, op, uint(width))
		return true
	}
	return false
}
