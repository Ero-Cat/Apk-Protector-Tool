package vmp

import (
	"fmt"
	"math/rand"
	"strings"

	"protector-tool/llvmwrap"
)

// Compiler 将 LLVM IR 函数编译为 VM 字节码
type Compiler struct {
	rand     *rand.Rand
	mapper   *OpcodeMapper
	buf      *BytecodeBuffer
	resolver *LabelResolver

	// 外部函数表：函数名 -> ID
	extFuncs  map[string]uint16
	nextExtID uint16

	// 局部变量表：Value 名 -> 槽位
	locals    map[string]uint8
	nextLocal uint8

	// 参数表：参数索引 -> 槽位
	params map[int]uint8
}

// NewCompiler 创建新的编译器实例
func NewCompiler(r *rand.Rand) *Compiler {
	// 生成随机 opcode 排列
	perm := r.Perm(256)
	mapper := NewOpcodeMapper(perm)

	return &Compiler{
		rand:      r,
		mapper:    mapper,
		buf:       NewBytecodeBuffer(mapper),
		resolver:  NewLabelResolver(),
		extFuncs:  make(map[string]uint16),
		nextExtID: 0,
		locals:    make(map[string]uint8),
		nextLocal: 0,
		params:    make(map[int]uint8),
	}
}

// CompileResult 包含编译结果
type CompileResult struct {
	Bytecode   []byte            // 编译后的字节码
	Mapper     *OpcodeMapper     // opcode 映射（运行时需要）
	ExtFuncs   map[string]uint16 // 外部函数表
	LocalCount int               // 局部变量数量
	ParamCount int               // 参数数量
}

// Compile 编译函数为字节码
func (c *Compiler) Compile(fn llvmwrap.Function) (*CompileResult, error) {
	// 重置编译状态
	c.buf = NewBytecodeBuffer(c.mapper)
	c.resolver = NewLabelResolver()
	c.locals = make(map[string]uint8)
	c.nextLocal = 0

	blocks := fn.BasicBlocks()
	if len(blocks) == 0 {
		return nil, fmt.Errorf("function has no basic blocks")
	}

	// 第一遍：收集所有基本块标签
	for _, bb := range blocks {
		// 标签将在第二遍中定义实际偏移
		_ = bb.Name()
	}

	// 第二遍：编译指令
	for _, bb := range blocks {
		bbName := bb.Name()
		if bbName == "" {
			bbName = fmt.Sprintf("bb_%d", c.buf.Len())
		}
		c.resolver.DefineLabel(bbName, c.buf.Len())

		for _, inst := range bb.Instructions() {
			if err := c.compileInstruction(inst); err != nil {
				return nil, fmt.Errorf("compile %s: %w", inst.Opcode(), err)
			}
		}
	}

	// 解析跳转标签
	if err := c.resolver.Resolve(c.buf); err != nil {
		return nil, fmt.Errorf("resolve labels: %w", err)
	}

	return &CompileResult{
		Bytecode:   c.buf.Bytes(),
		Mapper:     c.mapper,
		ExtFuncs:   c.extFuncs,
		LocalCount: int(c.nextLocal),
		ParamCount: len(c.params),
	}, nil
}

// compileInstruction 编译单条 LLVM IR 指令
func (c *Compiler) compileInstruction(inst llvmwrap.Instruction) error {
	opcode := inst.Opcode()

	switch opcode {
	// 算术运算
	case "add", "Add":
		c.compileOperands(inst)
		c.buf.Emit(OP_ADD)

	case "sub", "Sub":
		c.compileOperands(inst)
		c.buf.Emit(OP_SUB)

	case "mul", "Mul":
		c.compileOperands(inst)
		c.buf.Emit(OP_MUL)

	case "sdiv", "SDiv", "udiv", "UDiv":
		c.compileOperands(inst)
		c.buf.Emit(OP_DIV)

	case "srem", "SRem", "urem", "URem":
		c.compileOperands(inst)
		c.buf.Emit(OP_MOD)

	// 位运算
	case "and", "And":
		c.compileOperands(inst)
		c.buf.Emit(OP_AND)

	case "or", "Or":
		c.compileOperands(inst)
		c.buf.Emit(OP_OR)

	case "xor", "Xor":
		c.compileOperands(inst)
		c.buf.Emit(OP_XOR)

	case "shl", "Shl":
		c.compileOperands(inst)
		c.buf.Emit(OP_SHL)

	case "lshr", "LShr", "ashr", "AShr":
		c.compileOperands(inst)
		c.buf.Emit(OP_SHR)

	// 比较
	case "icmp", "ICmp":
		c.compileOperands(inst)
		// 根据比较类型选择 opcode
		// 简化处理：默认使用 EQ
		c.buf.Emit(OP_CMP_EQ)

	// 内存操作
	case "load", "Load":
		c.compileOperands(inst)
		c.buf.Emit(OP_LOAD)

	case "store", "Store":
		c.compileOperands(inst)
		c.buf.Emit(OP_STORE)

	// 控制流
	case "br", "Br":
		operands := inst.Operands()
		if len(operands) == 1 {
			// 无条件跳转
			c.buf.Emit(OP_JMP)
			// 记录回填位置
			patchOffset := c.buf.Len()
			c.buf.EmitU16(0) // 占位
			// 标签名从 operand 获取（简化：使用索引）
			c.resolver.AddBackpatch(Backpatch{
				Offset:    patchOffset,
				Target:    fmt.Sprintf("target_%d", len(c.resolver.backpatch)),
				Relative:  true,
				FromAfter: c.buf.Len(),
			})
		} else if len(operands) >= 3 {
			// 条件跳转
			c.compileValue(operands[0]) // 条件
			c.buf.Emit(OP_JNZ)
			patchOffset := c.buf.Len()
			c.buf.EmitU16(0)
			c.resolver.AddBackpatch(Backpatch{
				Offset:    patchOffset,
				Target:    fmt.Sprintf("true_%d", len(c.resolver.backpatch)),
				Relative:  true,
				FromAfter: c.buf.Len(),
			})
		}

	case "ret", "Ret":
		c.buf.Emit(OP_RET)

	// 函数调用
	case "call", "Call":
		operands := inst.Operands()
		if len(operands) > 0 {
			// 压入参数
			for i := len(operands) - 2; i >= 0; i-- {
				c.compileValue(operands[i])
			}
			// 获取或分配函数 ID
			fnName := "unknown"
			fnID := c.getOrAllocExtFunc(fnName)
			c.buf.Emit(OP_CALL_EXT)
			c.buf.EmitU16(fnID)
			c.buf.EmitU8(uint8(len(operands) - 1))
		}

	// 类型转换等（生成 NOP）
	case "bitcast", "BitCast", "trunc", "Trunc", "zext", "ZExt", "sext", "SExt":
		// 类型转换在 VM 中可能不需要显式处理
		c.buf.Emit(OP_NOP)

	// alloca（分配局部变量槽）
	case "alloca", "Alloca":
		// 分配槽位但不生成指令
		c.allocLocal()

	// 其他指令：生成 NOP
	default:
		if !strings.HasPrefix(opcode, "llvm.") {
			// 未知指令，生成 NOP
			c.buf.Emit(OP_NOP)
		}
	}

	return nil
}

// compileOperands 编译指令的所有操作数
func (c *Compiler) compileOperands(inst llvmwrap.Instruction) {
	for _, op := range inst.Operands() {
		c.compileValue(op)
	}
}

// compileValue 将 LLVM Value 编译为值压栈指令
func (c *Compiler) compileValue(v llvmwrap.Value) {
	if isConst, val := v.IsConstInt(); isConst {
		// 常量整数
		c.buf.Emit(OP_PUSH_CONST)
		c.buf.EmitU32(uint32(val))
	} else {
		// 变量或参数：压入局部变量槽
		slot := c.allocLocal()
		c.buf.Emit(OP_PUSH_LOCAL)
		c.buf.EmitU8(slot)
	}
}

// allocLocal 分配局部变量槽
func (c *Compiler) allocLocal() uint8 {
	slot := c.nextLocal
	c.nextLocal++
	return slot
}

// getOrAllocExtFunc 获取或分配外部函数 ID
func (c *Compiler) getOrAllocExtFunc(name string) uint16 {
	if id, ok := c.extFuncs[name]; ok {
		return id
	}
	id := c.nextExtID
	c.extFuncs[name] = id
	c.nextExtID++
	return id
}

// GetMapper 返回 opcode 映射器
func (c *Compiler) GetMapper() *OpcodeMapper {
	return c.mapper
}
