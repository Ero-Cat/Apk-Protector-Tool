package llvmwrap

// llvmwrap 提供一层面向 Go 的轻量封装，屏蔽直接调用 LLVM C API 的细节。
// 组成：
//   * native.go（需 `-tags llvm`）：通过 cgo 连接真实的 LLVM C 接口。
//   * mock.go（默认）：在无 LLVM 环境下返回 ErrNoLLVM，方便代码编译与单测。

import "errors"

// ErrNoLLVM：未启用原生 LLVM 绑定时的统一错误。
var ErrNoLLVM = errors.New("llvm support not built; rebuild with -tags llvm")

type Module struct{ impl moduleImpl }
type Function struct{ impl functionImpl }
type BasicBlock struct{ impl basicBlockImpl }
type Instruction struct{ impl instructionImpl }
type Builder struct{ impl builderImpl }

// 能力探测：是否包含原生 LLVM 绑定。
func HasNative() bool { return hasNativeImpl() }

// ParseBitcode 从 .bc/.ll 文件读取模块。
func ParseBitcode(path string) (*Module, error) { return parseBitcodeImpl(path) }

// WriteBitcode 将模块写回磁盘。
func (m *Module) WriteBitcode(path string) error { return m.impl.writeBitcode(path) }

// Dispose 释放底层 LLVM 资源（仅原生构建有效）。
func (m *Module) Dispose() { m.impl.dispose() }

func (m *Module) Functions() []Function { return m.impl.functions() }
func (m *Module) AddGlobalString(name string, data []byte) Value {
	return m.impl.addGlobalString(name, data)
}

func (f *Function) Name() string              { return f.impl.name() }
func (f *Function) BasicBlocks() []BasicBlock { return f.impl.basicBlocks() }
func (f *Function) AppendBasicBlock(name string) BasicBlock {
	return f.impl.appendBasicBlock(name)
}
func (f *Function) Type() ValueType       { return f.impl.typ() }
func (f *Function) ReturnType() ValueType { return f.impl.returnType() }

func (bb *BasicBlock) Instructions() []Instruction { return bb.impl.instructions() }
func (bb *BasicBlock) AppendInstructionBefore(before Instruction, inst Instruction) {
	bb.impl.appendInstructionBefore(before, inst)
}
func (bb *BasicBlock) TerminateWith(inst Instruction) { bb.impl.terminateWith(inst) }
func (bb *BasicBlock) MoveBefore(target BasicBlock)   { bb.impl.moveBefore(target) }
func (bb *BasicBlock) Name() string                   { return bb.impl.name() }

func (i *Instruction) Opcode() string { return i.impl.opcode() }
func (i *Instruction) ReplaceAllUsesWith(newInst Instruction) {
	i.impl.replaceAllUsesWith(newInst)
}
func (i Instruction) Operands() []Value { return i.impl.operands() }

func NewBuilderAt(instr Instruction) Builder { return Builder{impl: newBuilderAtImpl(instr)} }
func NewBuilderAtEnd(bb BasicBlock) Builder  { return Builder{impl: newBuilderAtEndImpl(bb)} }
func (b Builder) CreateCall(fn Function, args []Value) Instruction {
	return b.impl.createCall(fn, args)
}
func (b Builder) CreateAdd(lhs, rhs Value, name string) Instruction {
	return b.impl.createAdd(lhs, rhs, name)
}
func (b Builder) CreateXor(lhs, rhs Value, name string) Instruction {
	return b.impl.createXor(lhs, rhs, name)
}
func (b Builder) CreateSub(lhs, rhs Value, name string) Instruction {
	return b.impl.createSub(lhs, rhs, name)
}
func (b Builder) CreateICmpEq(lhs, rhs Value, name string) Instruction {
	return b.impl.createICmpEq(lhs, rhs, name)
}
func (b Builder) CreateBr(bb BasicBlock) Instruction { return b.impl.createBr(bb) }
func (b Builder) CreateCondBr(cond Value, t, f BasicBlock) Instruction {
	return b.impl.createCondBr(cond, t, f)
}
func (b Builder) CreateRetVoid() Instruction { return b.impl.createRetVoid() }
func (b Builder) CreateBitCast(v Value, dst ValueType, name string) Instruction {
	return b.impl.createBitCast(v, dst, name)
}

// CreateAlloca 在函数入口分配栈空间
func (b Builder) CreateAlloca(ty ValueType, name string) Value {
	return b.impl.createAlloca(ty, name)
}

// CreateLoad 从指针加载值
func (b Builder) CreateLoad(ty ValueType, ptr Value, name string) Value {
	return b.impl.createLoad(ty, ptr, name)
}

// CreateStore 向指针存储值
func (b Builder) CreateStore(val Value, ptr Value) Instruction {
	return b.impl.createStore(val, ptr)
}

// CreateSwitch 创建 switch 跳转
func (b Builder) CreateSwitch(cond Value, defaultBB BasicBlock, numCases int) SwitchInst {
	return b.impl.createSwitch(cond, defaultBB, numCases)
}

func (b Builder) Dispose()                          { b.impl.dispose() }
func (b Builder) SetInsertPointAtEnd(bb BasicBlock) { b.impl.setInsertPointAtEnd(bb) }

// Values are thin wrappers to share pointers across builders.
type Value struct{ impl valueImpl }

func (i Instruction) AsValue() Value       { return Value{impl: i.impl.asValue()} }
func (f Function) AsValue() Value          { return Value{impl: f.impl.asValue()} }
func (v Value) AsValue() Value             { return v }
func (v Value) IsConstInt() (bool, uint64) { return v.impl.isConstInt() }
func (v Value) Type() ValueType            { return v.impl.typ() }

// Constants
func ConstInt(value int64, bits uint) Value { return constIntImpl(value, bits) }
func ConstBool(v bool) Value                { return constBoolImpl(v) }

// Function/Global helpers
func (m *Module) EnsureFunction(name string, ret ValueType, params []ValueType) Function {
	return m.impl.ensureFunction(name, ret, params)
}

// Types are small descriptors used by builders.
type ValueType struct{ impl valueTypeImpl }

func IntType(bits uint) ValueType          { return intTypeImpl(bits) }
func VoidType() ValueType                  { return voidTypeImpl() }
func PointerType(elem ValueType) ValueType { return pointerTypeImpl(elem) }
func (t ValueType) IsZero() bool           { return t.impl.isZero() }
func (t ValueType) Equal(o ValueType) bool { return t.impl.equal(o.impl) }
func (t ValueType) IntWidth() (int, bool)  { return t.impl.intWidth() }

// SwitchInst 表示 switch 指令
type SwitchInst struct{ impl switchInstImpl }

// AddCase 向 switch 添加 case
func (s SwitchInst) AddCase(val Value, dest BasicBlock) {
	s.impl.addCase(val, dest)
}
