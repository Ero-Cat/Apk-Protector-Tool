package vmp

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/Ero-Cat/Apk-Protector-Tool/llvmwrap"
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

	// 基本块标签表：块 Value RefID -> 稳定标签名
	blockNames map[uintptr]string

	// def-use 槽位表：产生值的指令 RefID -> 结果槽号
	defSlots map[uintptr]uint8
}

// NewCompiler 创建新的编译器实例
func NewCompiler(r *rand.Rand) *Compiler {
	// 生成随机 opcode 排列
	perm := r.Perm(256)
	mapper := NewOpcodeMapper(perm)

	return &Compiler{
		rand:       r,
		mapper:     mapper,
		buf:        NewBytecodeBuffer(mapper),
		resolver:   NewLabelResolver(),
		extFuncs:   make(map[string]uint16),
		nextExtID:  0,
		locals:     make(map[string]uint8),
		nextLocal:  0,
		params:     make(map[int]uint8),
		blockNames: make(map[uintptr]string),
		defSlots:   make(map[uintptr]uint8),
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

// Compile 编译函数为字节码。
//
// 值模型：每个产生值的 SSA 指令绑定一个局部槽——操作数按定义指令解析为
// PUSH_LOCAL（或常量 PUSH_CONST），指令执行后 STORE_LOCAL 写回自己的槽。
// 访存/调用/phi 等当前不支持的指令直接报错，由上层跳过该函数（保守策略：
// 要么语义正确地虚拟化，要么明确不虚拟化，绝不产出算错的字节码）。
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

	// 第一遍：块标签表（跳转回填目标）+ 指令结果槽位表（def-use 绑定）。
	c.blockNames = make(map[uintptr]string)
	c.defSlots = make(map[uintptr]uint8)
	for i, bb := range blocks {
		c.blockNames[bb.AsValue().RefID()] = blockLabel(bb, i)
		for _, inst := range bb.Instructions() {
			if producesValue(inst.Opcode()) {
				c.defSlots[inst.AsValue().RefID()] = c.allocLocal()
			}
		}
	}

	// 第二遍：编译指令
	for _, bb := range blocks {
		c.resolver.DefineLabel(c.blockNames[bb.AsValue().RefID()], c.buf.Len())

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

// producesValue 报告指令是否产生可绑槽的 SSA 结果。
func producesValue(opcode string) bool {
	switch opcode {
	case "add", "sub", "mul", "sdiv", "udiv", "srem", "urem",
		"and", "or", "xor", "shl", "lshr", "ashr",
		"icmp",
		"trunc", "zext", "sext", "bitcast":
		return true
	}
	return false
}

// blockLabel returns the label for a basic block: its LLVM name when it has
// one, otherwise a stable positional name.
func blockLabel(bb llvmwrap.BasicBlock, index int) string {
	if name := bb.Name(); name != "" {
		return name
	}
	return fmt.Sprintf("bb%d", index)
}

// compileInstruction 编译单条 LLVM IR 指令。不支持的指令返回错误，
// 由上层保守跳过整个函数。
func (c *Compiler) compileInstruction(inst llvmwrap.Instruction) error {
	opcode := inst.Opcode()
	slot := c.defSlots[inst.AsValue().RefID()]

	switch opcode {
	// 算术与位运算：解析操作数 -> 运算 -> 结果写回槽。
	case "add", "sub", "mul", "sdiv", "udiv", "srem", "urem",
		"and", "or", "xor", "shl", "lshr", "ashr":
		if err := c.pushOperands(inst); err != nil {
			return err
		}
		c.buf.Emit(binaryOpcode(opcode))
		c.emitStoreLocal(slot)

	// 比较
	case "icmp":
		if err := c.pushOperands(inst); err != nil {
			return err
		}
		op, err := icmpOpcode(inst.ICmpPredicate())
		if err != nil {
			return err
		}
		c.buf.Emit(op)
		c.emitStoreLocal(slot)

	// 类型转换：值语义不变，等价于把操作数复制进结果槽。
	case "trunc", "zext", "sext", "bitcast":
		ops := inst.Operands()
		if len(ops) != 1 {
			return fmt.Errorf("cast with %d operands", len(ops))
		}
		if err := c.pushValue(ops[0]); err != nil {
			return err
		}
		c.emitStoreLocal(slot)

	// 控制流
	case "br":
		operands := inst.Operands()
		if len(operands) == 1 {
			target, err := c.branchTarget(operands[0])
			if err != nil {
				return err
			}
			c.emitJump(OP_JMP, target)
		} else if len(operands) >= 3 {
			cond, tTarget, fTarget := operands[0], operands[1], operands[2]
			tName, err := c.branchTarget(tTarget)
			if err != nil {
				return err
			}
			fName, err := c.branchTarget(fTarget)
			if err != nil {
				return err
			}
			if err := c.pushValue(cond); err != nil {
				return err
			}
			c.emitJump(OP_JNZ, tName)
			c.emitJump(OP_JMP, fName)
		} else {
			return fmt.Errorf("br with %d operands", len(operands))
		}

	case "ret":
		if len(inst.Operands()) != 0 {
			return fmt.Errorf("non-void ret not supported")
		}
		c.buf.Emit(OP_RET)

	default:
		if strings.HasPrefix(opcode, "llvm.") {
			return nil // intrinsic 调用：跳过
		}
		// load/store/alloca/call/phi/select/…：值模型或 ABI 尚未支持。
		return fmt.Errorf("unsupported instruction %q (skipping function)", opcode)
	}

	return nil
}

// binaryOpcode 把 LLVM 二元运算映射到 VM 指令。
func binaryOpcode(opcode string) Opcode {
	switch opcode {
	case "add":
		return OP_ADD
	case "sub":
		return OP_SUB
	case "mul":
		return OP_MUL
	case "sdiv", "udiv":
		return OP_DIV
	case "srem", "urem":
		return OP_MOD
	case "and":
		return OP_AND
	case "or":
		return OP_OR
	case "xor":
		return OP_XOR
	case "shl":
		return OP_SHL
	default: // lshr, ashr
		return OP_SHR
	}
}

// pushOperands 依次压入指令的全部操作数。
func (c *Compiler) pushOperands(inst llvmwrap.Instruction) error {
	for _, op := range inst.Operands() {
		if err := c.pushValue(op); err != nil {
			return err
		}
	}
	return nil
}

// pushValue 把一个 LLVM 值压栈：常量取立即数，SSA 值取其定义槽，
// 其他（函数参数/全局）在当前 ABI 下不支持。
func (c *Compiler) pushValue(v llvmwrap.Value) error {
	if isConst, val := v.IsConstInt(); isConst {
		c.buf.Emit(OP_PUSH_CONST)
		c.buf.EmitU32(uint32(val))
		return nil
	}
	if slot, ok := c.defSlots[v.RefID()]; ok {
		c.buf.Emit(OP_PUSH_LOCAL)
		c.buf.EmitU8(slot)
		return nil
	}
	return fmt.Errorf("operand is a function argument or global (unsupported until the VM entry ABI passes arguments, see ROADMAP P2.3)")
}

// emitStoreLocal 弹出栈顶写入槽位。
func (c *Compiler) emitStoreLocal(slot uint8) {
	c.buf.Emit(OP_STORE_LOCAL)
	c.buf.EmitU8(slot)
}

// icmpOpcode maps an LLVM integer comparison predicate to a VM comparison
// opcode. Empty predicate falls back to EQ to stay usable on wrappers that
// cannot report predicates.
func icmpOpcode(predicate string) (Opcode, error) {
	switch predicate {
	case "", "eq":
		return OP_CMP_EQ, nil
	case "ne":
		return OP_CMP_NE, nil
	case "sgt":
		return OP_CMP_GT, nil
	case "sge":
		return OP_CMP_GE, nil
	case "slt":
		return OP_CMP_LT, nil
	case "sle":
		return OP_CMP_LE, nil
	case "ugt":
		return OP_CMP_UGT, nil
	case "uge":
		return OP_CMP_UGE, nil
	case "ult":
		return OP_CMP_ULT, nil
	case "ule":
		return OP_CMP_ULE, nil
	}
	return 0, fmt.Errorf("unsupported icmp predicate %q", predicate)
}

// branchTarget resolves a branch operand to the label of its destination
// block, using the pre-pass name table for anonymous blocks.
func (c *Compiler) branchTarget(v llvmwrap.Value) (string, error) {
	if name := v.Name(); name != "" {
		return name, nil
	}
	if name, ok := c.blockNames[v.RefID()]; ok {
		return name, nil
	}
	return "", fmt.Errorf("branch target is not a known basic block")
}

// emitJump writes a relative jump and registers its backpatch against the
// target label.
func (c *Compiler) emitJump(op Opcode, target string) {
	c.buf.Emit(op)
	patchOffset := c.buf.Len()
	c.buf.EmitU16(0) // 占位，Resolve 阶段回填
	c.resolver.AddBackpatch(Backpatch{
		Offset:    patchOffset,
		Target:    target,
		Relative:  true,
		FromAfter: c.buf.Len(),
	})
}

// allocLocal 分配局部变量槽
func (c *Compiler) allocLocal() uint8 {
	slot := c.nextLocal
	c.nextLocal++
	return slot
}

// GetMapper 返回 opcode 映射器
func (c *Compiler) GetMapper() *OpcodeMapper {
	return c.mapper
}
