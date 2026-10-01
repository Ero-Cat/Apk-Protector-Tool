package passes

import (
	"encoding/json"
	"fmt"
	"math/rand"

	"github.com/Ero-Cat/Apk-Protector-Tool/config"
	"github.com/Ero-Cat/Apk-Protector-Tool/llvmwrap"
	"github.com/Ero-Cat/Apk-Protector-Tool/passes/vmp"
	"github.com/Ero-Cat/Apk-Protector-Tool/report"
)

// VirtualizePass：把符合条件的函数编译为 VM 字节码，原函数体整体替换为
// 对统一 VM 入口的调用桩（ROADMAP P2.3：非 void 返回 + 参数传递 + 旧体擦除）。
// 解释器/VM 的运行时实现在 runtime/src/vm_entry.c。
type VirtualizePass struct {
	Cfg        *config.Config
	Rand       *rand.Rand
	Report     *report.Report
	compilers  map[string]*vmp.Compiler // VM 名 -> 编译器
	buildNonce uint64                   // 本次构建随机化标识
}

func (p *VirtualizePass) Name() string { return "virtualize" }

// 统一入口符号：i32 (i8* bytecode, i8* meta, i32 a0..a3)。
// VM 选择已经烘焙进字节码的随机化 opcode 映射与 key_pad——多 VM 的差异
// 体现在数据里而不是符号上，自定义 VM 名不再制造链接期陷阱。
const vmEntrySymbol = "__goprotect_vm_entry_encrypted"

func (p *VirtualizePass) Run(m *llvmwrap.Module) error {
	// 初始化每个 VM 的编译器
	if p.compilers == nil {
		p.compilers = make(map[string]*vmp.Compiler)
		p.buildNonce = uint64(p.Rand.Int63())
		for _, vm := range p.Cfg.VMP.VMs {
			p.compilers[vm.Name] = vmp.NewCompiler(p.Rand)
		}
		p.Report.AddMessage(fmt.Sprintf("vmp: initialized %d VM compilers, build_nonce=%d", len(p.compilers), p.buildNonce))
	}

	ptrType := llvmwrap.PointerType(llvmwrap.IntType(8))
	i32 := llvmwrap.IntType(32)
	entryTy := []llvmwrap.ValueType{ptrType, ptrType, i32, i32, i32, i32}
	entry := m.EnsureFunction(vmEntrySymbol, i32, entryTy)

	virtualizedCount := 0
	for _, fn := range m.Functions() {
		if skipFunction(fn.Name(), p.Cfg) {
			continue
		}
		if !shouldRunByRatio(p.Rand, p.Cfg.Obfuscation.VirtualizeRatio) {
			continue
		}
		vmName := p.pickVMFor(fn.Name())
		retTy := fn.ReturnType()
		if retTy.IsZero() {
			continue // 声明：无函数体
		}
		// ABI v2 资格：返回 void 或 i32；参数全部 i32 且不超过 4 个
		// （VM 栈是 i32 宽，i64 会截断——保守跳过而非算错）。
		retVoid := retTy.Equal(llvmwrap.VoidType())
		if w, ok := retTy.IntWidth(); !retVoid && (!ok || w != 32) {
			continue
		}
		if n := fn.ParamCount(); n > 4 {
			continue
		}
		eligible := true
		for i := 0; i < fn.ParamCount(); i++ {
			if w, ok := fn.Param(i).Type().IntWidth(); !ok || w != 32 {
				eligible = false
				break
			}
		}
		if !eligible {
			continue
		}
		bbs := fn.BasicBlocks()
		if len(bbs) == 0 {
			continue
		}

		// 使用真实编译器编译函数
		compiler := p.compilers[vmName]
		if compiler == nil {
			compiler = vmp.NewCompiler(p.Rand)
			p.compilers[vmName] = compiler
		}

		result, err := compiler.Compile(fn)
		if err != nil {
			p.Report.AddMessage(fmt.Sprintf("vmp: skip %s: compile error: %v", fn.Name(), err))
			continue
		}

		// 加密字节码
		enc, keyPad := p.encryptBytecode(result.Bytecode)
		gv := m.AddGlobalString("__gp_bc_"+fn.Name(), enc)

		// 元数据：记录 VM 名、build nonce、密钥片段、opcode 解码表与外部函数表。
		metaStr := []byte(p.metadataJSON(fn.Name(), vmName, keyPad, result))
		metaGv := m.AddGlobalString("__gp_bc_meta_"+fn.Name(), metaStr)

		// 新函数体：调用统一入口并回传返回值；参数补零到 4 槽。
		args := make([]llvmwrap.Value, 0, 6)
		newEntry := fn.AppendBasicBlock("gp.vm.entry")
		builder := llvmwrap.NewBuilderAtEnd(newEntry)
		bcPtr := builder.CreateBitCast(gv.AsValue(), ptrType, "bcptr")
		metaPtr := builder.CreateBitCast(metaGv.AsValue(), ptrType, "metaptr")
		args = append(args, bcPtr.AsValue(), metaPtr.AsValue())
		for i := 0; i < 4; i++ {
			if i < fn.ParamCount() {
				args = append(args, fn.Param(i))
			} else {
				args = append(args, llvmwrap.ConstInt(0, 32))
			}
		}
		call := builder.CreateCall(entry, args)
		if retVoid {
			builder.CreateRetVoid()
		} else {
			builder.CreateRet(call.AsValue())
		}
		builder.Dispose()

		// 旧函数体整体擦除（P2.3）：先逆序抹掉全部指令（使用者先于定义，
		// phi 已在编译期排除保证支配序成立），再删除腾空的基本块。新体
		// 成为唯一块，原指令不再残留在产物里。
		for i := len(bbs) - 1; i >= 0; i-- {
			insts := bbs[i].Instructions()
			for j := len(insts) - 1; j >= 0; j-- {
				insts[j].EraseFromParent()
			}
		}
		for i := len(bbs) - 1; i >= 0; i-- {
			bbs[i].Delete()
		}

		p.Report.MarkVirtualized(fn.Name())
		virtualizedCount++
	}

	p.Report.AddMessage(fmt.Sprintf("vmp: virtualized %d functions", virtualizedCount))
	return nil
}

// encryptBytecode：按单字节 XOR 方式加密。返回密文与随机片段 keyPad——
// 最终密钥为 staticKeyByte() ^ keyPad，静态片段不落盘，运行时经
// goprotect_set_static_key 注册后与元数据中的 keyPad 组合还原密钥。
func (p *VirtualizePass) encryptBytecode(bc []byte) ([]byte, byte) {
	keyPad := byte(p.Rand.Intn(256))
	key := p.staticKeyByte() ^ keyPad
	out := make([]byte, len(bc))
	for i, b := range bc {
		out[i] = b ^ key
	}
	return out, keyPad
}

// staticKeyByte：静态密钥片段（配置 StaticKey 的末字节，默认 0x5A）。
func (p *VirtualizePass) staticKeyByte() byte {
	if len(p.Cfg.VMP.StaticKey) > 0 {
		return p.Cfg.VMP.StaticKey[len(p.Cfg.VMP.StaticKey)-1]
	}
	return 0x5a
}

// pickVMFor：根据函数名与级别选择 VM。
func (p *VirtualizePass) pickVMFor(fn string) string {
	if p.Cfg.VMP.Levels.FunctionVM != nil {
		if vm, ok := p.Cfg.VMP.Levels.FunctionVM[fn]; ok && vm != "" {
			return vm
		}
	}
	for _, n := range p.Cfg.VMP.Levels.CriticalList {
		if n == fn {
			return p.Cfg.VMP.Levels.Critical
		}
	}
	for _, n := range p.Cfg.VMP.Levels.SensitiveList {
		if n == fn {
			return p.Cfg.VMP.Levels.Sensitive
		}
	}
	return p.Cfg.VMP.Levels.Normal
}

// bytecodeMetadata 字节码元数据结构。运行时依赖其中的 bytecode_len、
// key_pad 与 opcodes 解码表才能执行（见 runtime/src/vm_entry.c）。
type bytecodeMetadata struct {
	Function    string           `json:"fn"`
	VM          string           `json:"vm"`
	BuildNonce  uint64           `json:"build_nonce"`
	BytecodeLen int              `json:"bytecode_len"`
	LocalCount  int              `json:"local_count"`
	ParamCount  int              `json:"param_count"`
	Encrypted   bool             `json:"encrypted"`
	KeyPad      uint8            `json:"key_pad"`
	KeyHint     string           `json:"key_hint,omitempty"`
	Opcodes     map[string]uint8 `json:"opcodes"`
}

// metadataJSON：生成结构化元数据 JSON。Opcodes 为"标准助记符 -> 本次构建
// 随机化字节"映射，运行时反转为解码表。
func (p *VirtualizePass) metadataJSON(fn, vm string, keyPad byte, result *vmp.CompileResult) string {
	opcodes := make(map[string]uint8, len(vmp.BaseOpcodes))
	for op, info := range vmp.BaseOpcodes {
		opcodes[info.Name] = byte(result.Mapper.Encode(op))
	}
	meta := bytecodeMetadata{
		Function:    fn,
		VM:          vm,
		BuildNonce:  p.buildNonce,
		BytecodeLen: len(result.Bytecode),
		LocalCount:  result.LocalCount,
		ParamCount:  result.ParamCount,
		Encrypted:   true,
		KeyPad:      keyPad,
		KeyHint:     p.Cfg.VMP.RuntimeKeyHint,
		Opcodes:     opcodes,
	}
	data, _ := json.Marshal(meta)
	return string(data)
}
