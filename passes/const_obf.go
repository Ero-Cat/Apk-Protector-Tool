package passes

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/Ero-Cat/Apk-Protector-Tool/config"
	"github.com/Ero-Cat/Apk-Protector-Tool/llvmwrap"
	"github.com/Ero-Cat/Apk-Protector-Tool/report"
)

// ConstObfPass：字面量混淆（ROADMAP P2.2）。
//   - 整数常量：按 substitute_intensity 概率把指令的常量整型操作数替换为
//     运行时可恢复的恒等表达式——xor 恒等链 (c^k)^k 或加法拆分 (c−k)+k；
//   - 字符串常量：私有 const i8 数组全局（使用者全为指令）替换为可写密文全局，
//     运行时经 __goprotect_decrypt_strings 按 __gp_str_regions 表原位解密。
type ConstObfPass struct {
	Cfg    *config.Config
	Rand   *rand.Rand
	Report *report.Report
}

func (p *ConstObfPass) Name() string { return "const-obf" }

// noWrapOpcodes 列出不做操作数包裹的指令：终结器中 switch 的 case 值必须是
// 常量（改写会破坏结构）；phi 的入参只需支配对应入边，在 phi 所在块前插入的
// 重写指令不保证支配关系，会产生非法 IR。
var noWrapOpcodes = map[string]bool{
	"ret": true, "br": true, "switch": true, "indirectbr": true,
	"invoke": true, "callbr": true, "unreachable": true, "fence": true,
	"phi": true,
}

func (p *ConstObfPass) Run(m *llvmwrap.Module) error {
	encrypted := p.encryptStringGlobals(m)

	decFn := m.EnsureFunction("__goprotect_decrypt_strings", llvmwrap.VoidType(), nil)

	intensity := p.Cfg.Obfuscation.SubstituteIntensity
	wrapped := 0
	for _, fn := range m.Functions() {
		if skipFunction(fn.Name(), p.Cfg) {
			continue
		}
		blocks := fn.BasicBlocks()
		if len(blocks) == 0 {
			continue
		}
		insts := blocks[0].Instructions()
		if len(insts) > 0 {
			// 入口一次性解密：按区域表原位 XOR 还原字符串密文。
			b := llvmwrap.NewBuilderAt(insts[0])
			b.CreateCall(decFn, nil)
			b.Dispose()
		}

		ks := newKeySlots(insts[0])
		fnWrapped := false
		for _, bb := range blocks {
			for _, inst := range bb.Instructions() {
				if noWrapOpcodes[inst.Opcode()] {
					continue
				}
				if intensity <= 0 || p.Rand.Intn(10) >= intensity {
					continue
				}
				if p.wrapConstantOperands(ks, inst) {
					wrapped++
					fnWrapped = true
				}
			}
		}
		if fnWrapped {
			p.Report.MarkObfuscated(fn.Name(), p.Name())
		}
	}

	p.Report.AddMessage(fmt.Sprintf(
		"const-obf: wrapped %d constant uses, encrypted %d string globals", wrapped, encrypted))
	return nil
}

// wrapConstantOperands 把指令上的全部常量整型操作数替换为恒等表达式。
// 密钥经栈槽中转以避开 IRBuilder 的常数折叠（见 wrap.go）。
func (p *ConstObfPass) wrapConstantOperands(ks *keySlots, inst llvmwrap.Instruction) bool {
	wrapped := false
	for idx, op := range inst.Operands() {
		if isConst, _ := op.IsConstInt(); !isConst {
			continue
		}
		width, ok := op.Type().IntWidth()
		if !ok {
			continue
		}
		wrapOperand(p.Rand, ks, inst, idx, op, uint(width))
		wrapped = true
	}
	return wrapped
}

// encryptStringGlobals 把可安全改写的私有字符串全局替换为加密可写全局，
// 返回加密区域数。仅当"所有使用者均为指令"才改写——被常量表达式引用的
// 全局（例如嵌进其他初始化器的 GEP）无法做指令级替换，整组跳过。
func (p *ConstObfPass) encryptStringGlobals(m *llvmwrap.Module) int {
	regions := []llvmwrap.StrRegion{}
	for _, g := range m.Globals() {
		name := g.Name()
		if strings.HasPrefix(name, "__gp_") || strings.HasPrefix(name, "__goprotect_") {
			continue
		}
		if !g.IsPrivateLinkage() || !g.IsGlobalConstant() {
			continue
		}
		data, ok := g.Initializer().ConstantDataArrayBytes()
		if !ok || len(data) == 0 {
			continue
		}
		users := g.Users()
		if len(users) == 0 {
			continue
		}
		allInstructions := true
		for _, u := range users {
			if !u.IsInstruction() {
				allInstructions = false
				break
			}
		}
		if !allInstructions {
			continue
		}

		key := uint8(p.Rand.Intn(255)) + 1
		enc := make([]byte, len(data))
		for i, b := range data {
			enc[i] = b ^ key
		}
		twin := m.AddGlobalBytes(name+".enc", enc)
		for _, u := range users {
			inst := u.AsInstruction()
			for idx, op := range inst.Operands() {
				if op.RefID() == g.RefID() {
					inst.SetOperand(idx, twin)
				}
			}
		}
		// 原明文全局已无使用者：立即删除，密文才是模块里唯一的字符串数据。
		g.DeleteGlobal()
		regions = append(regions, llvmwrap.StrRegion{Data: twin, Length: int32(len(data)), Key: key})
	}
	m.EmitStrRegionsTable("__gp_str_regions", regions)
	return len(regions)
}
