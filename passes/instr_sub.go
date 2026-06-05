package passes

import (
	"math/rand"
	"protector-tool/config"
	"protector-tool/llvmwrap"
	"protector-tool/report"
)

// InstrSubPass：用可逆的 xor 包裹指令结果，达到轻量替换效果。
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
				if inst.Opcode() == "ret" {
					continue
				}
				// Apply with probability derived from intensity.
				if p.Rand.Intn(10) >= intensity {
					continue
				}
				// Wrap the value with xor key.
				key := int64(p.Rand.Int63n(1<<16) + 1)
				builder := llvmwrap.NewBuilderAt(inst)
				kVal := llvmwrap.ConstInt(key, 64)
				tmp := builder.CreateXor(inst.AsValue(), kVal, "sub.x1")
				tmp2 := builder.CreateXor(tmp.AsValue(), kVal, "sub.x2")
				inst.ReplaceAllUsesWith(tmp2)
				builder.Dispose()
				changed = true
			}
		}
		if changed {
			p.Report.MarkObfuscated(fn.Name(), p.Name())
		}
	}
	return nil
}
