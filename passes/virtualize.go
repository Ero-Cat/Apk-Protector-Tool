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

// VirtualizePass：将部分 void 函数替换为 VM 入口跳板，并把生成的字节码以全局数组形式存入模块。
// 解释器/VM 的运行时实现需要在 Android 侧单独提供。
type VirtualizePass struct {
	Cfg        *config.Config
	Rand       *rand.Rand
	Report     *report.Report
	compilers  map[string]*vmp.Compiler // VM 名 -> 编译器
	buildNonce uint64                   // 本次构建随机化标识
}

func (p *VirtualizePass) Name() string { return "virtualize" }

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

	// 为每个 VM 生成对应入口符号，便于运行时区分。
	// 入口接收 (bytecode, meta) 双参数：运行时需要元数据里的长度、密钥
	// 片段与 opcode 解码表才能执行随机化后的字节码。
	ptrType := llvmwrap.PointerType(llvmwrap.IntType(8))
	vmEntryFns := map[string]llvmwrap.Function{}
	for _, vm := range p.Cfg.VMP.VMs {
		name := "__goprotect_vm_entry_encrypted_" + vm.Name
		vmEntryFns[vm.Name] = m.EnsureFunction(name, llvmwrap.VoidType(), []llvmwrap.ValueType{ptrType, ptrType})
	}

	virtualizedCount := 0
	for _, fn := range m.Functions() {
		if skipFunction(fn.Name(), p.Cfg) {
			continue
		}
		if !shouldRunByRatio(p.Rand, p.Cfg.Obfuscation.VirtualizeRatio) {
			continue
		}
		vmName := p.pickVMFor(fn.Name())
		entry, ok := vmEntryFns[vmName]
		if !ok {
			// 未定义的 VM 名，退回默认。
			vmName = p.Cfg.VMP.Levels.Normal
			entry = vmEntryFns[vmName]
		}
		// Only handle void functions for now to keep ABI simple.
		if fn.ReturnType().IsZero() {
			continue
		}
		if !fn.ReturnType().Equal(llvmwrap.VoidType()) {
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

		// 新入口：调用对应 VM 的加密入口（字节码 + 元数据双参数）。
		newEntry := fn.AppendBasicBlock("gp.vm.entry")
		builder := llvmwrap.NewBuilderAtEnd(newEntry)
		bcPtr := builder.CreateBitCast(gv.AsValue(), ptrType, "bcptr")
		metaPtr := builder.CreateBitCast(metaGv.AsValue(), ptrType, "metaptr")
		builder.CreateCall(entry, []llvmwrap.Value{bcPtr.AsValue(), metaPtr.AsValue()})
		builder.CreateRetVoid()
		builder.Dispose()

		// Make it the first block.
		newEntry.MoveBefore(bbs[0])

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
	Function    string            `json:"fn"`
	VM          string            `json:"vm"`
	BuildNonce  uint64            `json:"build_nonce"`
	BytecodeLen int               `json:"bytecode_len"`
	LocalCount  int               `json:"local_count"`
	ParamCount  int               `json:"param_count"`
	Encrypted   bool              `json:"encrypted"`
	KeyPad      uint8             `json:"key_pad"`
	KeyHint     string            `json:"key_hint,omitempty"`
	Opcodes     map[string]uint8  `json:"opcodes"`
	ExtFuncs    map[string]uint16 `json:"ext_funcs,omitempty"`
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
		ExtFuncs:    result.ExtFuncs,
	}
	data, _ := json.Marshal(meta)
	return string(data)
}
