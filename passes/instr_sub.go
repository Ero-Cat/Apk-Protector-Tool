package passes

import (
	"math/rand"

	"github.com/Ero-Cat/Apk-Protector-Tool/config"
	"github.com/Ero-Cat/Apk-Protector-Tool/llvmwrap"
	"github.com/Ero-Cat/Apk-Protector-Tool/report"
)

// wrapIntegerOperand 用 xor(xor(op, k), k) 恒等链包裹 inst 的第一个整型操作数，
// 并仅替换该操作数（SetOperand）。相比包裹"指令结果 + ReplaceAllUsesWith"：
//   - 只触碰当前指令，不会把新插入指令的操作数改写回自身（自引用环）；
//   - 整型过滤避免对指针/void 做 xor（opaque 指针下即非法 IR）。
func wrapIntegerOperand(r *rand.Rand, inst llvmwrap.Instruction) bool {
	for idx, op := range inst.Operands() {
		width, ok := op.Type().IntWidth()
		if !ok {
			continue
		}
		key := int64(r.Intn(1<<8) + 1)
		k := llvmwrap.ConstInt(key, uint(width))
		b := llvmwrap.NewBuilderAt(inst)
		x1 := b.CreateXor(op, k, "wrap.x1")
		x2 := b.CreateXor(x1.AsValue(), k, "wrap.x2")
		b.Dispose()
		inst.SetOperand(idx, x2.AsValue())
		return true
	}
	return false
}

// InstrSubPass：用可逆的 xor 恒等链包裹指令的整型操作数，达到轻量替换效果。
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
		changed := false
		for _, bb := range fn.BasicBlocks() {
			for _, inst := range bb.Instructions() {
				if inst.Opcode() == "ret" || inst.Opcode() == "br" {
					continue
				}
				// Apply with probability derived from intensity.
				if p.Rand.Intn(10) >= intensity {
					continue
				}
				if wrapIntegerOperand(p.Rand, inst) {
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
