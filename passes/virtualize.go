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
	vmEntryFns := map[string]llvmwrap.Function{}
	for _, vm := range p.Cfg.VMP.VMs {
		name := "__goprotect_vm_entry_encrypted_" + vm.Name
		vmEntryFns[vm.Name] = m.EnsureFunction(name, llvmwrap.VoidType(), []llvmwrap.ValueType{llvmwrap.PointerType(llvmwrap.IntType(8))})
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
		enc, keyByte := p.encryptBytecode(result.Bytecode)
		gv := m.AddGlobalString("__gp_bc_"+fn.Name(), enc)

		// 元数据：记录使用的 VM 名、build nonce、key 线索、外部函数表。
		metaStr := []byte(p.metadataJSON(fn.Name(), vmName, keyByte, result))
		_ = m.AddGlobalString("__gp_bc_meta_"+fn.Name(), metaStr)

		// 新入口：调用对应 VM 的加密入口。
		newEntry := fn.AppendBasicBlock("gp.vm.entry")
		builder := llvmwrap.NewBuilderAtEnd(newEntry)
		ptr := builder.CreateBitCast(gv.AsValue(), llvmwrap.PointerType(llvmwrap.IntType(8)), "bcptr")
		builder.CreateCall(entry, []llvmwrap.Value{ptr.AsValue()})
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

// encryptBytecode：按单字节 XOR 方式加密，key 由静态片段与随机片段组成。
func (p *VirtualizePass) encryptBytecode(bc []byte) ([]byte, byte) {
	key := p.deriveKeyByte()
	out := make([]byte, len(bc))
	for i, b := range bc {
		out[i] = b ^ key
	}
	return out, key
}

// deriveKeyByte：将静态 key 取最低 8bit，与随机片段异或得到最终 key。
func (p *VirtualizePass) deriveKeyByte() byte {
	static := byte(0x5a)
	if len(p.Cfg.VMP.StaticKey) > 0 {
		static = p.Cfg.VMP.StaticKey[len(p.Cfg.VMP.StaticKey)-1]
	}
	randByte := byte(p.Rand.Intn(256))
	return static ^ randByte
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

// bytecodeMetadata 字节码元数据结构
type bytecodeMetadata struct {
	Function    string            `json:"fn"`
	VM          string            `json:"vm"`
	BuildNonce  uint64            `json:"build_nonce"`
	KeyHint     string            `json:"key_hint"`
	BytecodeLen int               `json:"bytecode_len"`
	LocalCount  int               `json:"local_count"`
	ExtFuncs    map[string]uint16 `json:"ext_funcs,omitempty"`
}

// metadataJSON：生成结构化元数据 JSON。
func (p *VirtualizePass) metadataJSON(fn, vm string, key byte, result *vmp.CompileResult) string {
	meta := bytecodeMetadata{
		Function:    fn,
		VM:          vm,
		BuildNonce:  p.buildNonce,
		KeyHint:     p.Cfg.VMP.RuntimeKeyHint,
		BytecodeLen: len(result.Bytecode),
		LocalCount:  result.LocalCount,
		ExtFuncs:    result.ExtFuncs,
	}
	data, _ := json.Marshal(meta)
	return string(data)
}
