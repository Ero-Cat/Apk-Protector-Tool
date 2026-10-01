//go:build !llvm

package llvmwrap

// Mock 实现：未携带 `-tags llvm` 时编译，所有操作直接返回 ErrNoLLVM。

type moduleImpl struct{}
type functionImpl struct{}
type basicBlockImpl struct{}
type instructionImpl struct{}
type builderImpl struct{}
type valueImpl struct{}
type valueTypeImpl struct{}

func hasNativeImpl() bool                                                    { return false }
func parseBitcodeImpl(string) (*Module, error)                               { return nil, ErrNoLLVM }
func (m moduleImpl) writeBitcode(string) error                               { return ErrNoLLVM }
func (m moduleImpl) dispose()                                                {}
func (m moduleImpl) functions() []Function                                   { return nil }
func (m moduleImpl) addGlobalString(string, []byte) Value                    { return Value{} }
func (f functionImpl) name() string                                          { return "" }
func (f functionImpl) basicBlocks() []BasicBlock                             { return nil }
func (f functionImpl) appendBasicBlock(string) BasicBlock                    { return BasicBlock{} }
func (f functionImpl) typ() ValueType                                        { return ValueType{} }
func (f functionImpl) returnType() ValueType                                 { return ValueType{} }
func (f functionImpl) asValue() valueImpl                                    { return valueImpl{} }
func (bb basicBlockImpl) instructions() []Instruction                        { return nil }
func (bb basicBlockImpl) appendInstructionBefore(Instruction, Instruction)   {}
func (bb basicBlockImpl) terminateWith(Instruction)                          {}
func (bb basicBlockImpl) moveBefore(BasicBlock)                              {}
func (bb basicBlockImpl) name() string                                       { return "" }
func (i instructionImpl) opcode() string                                     { return "" }
func (i instructionImpl) replaceAllUsesWith(Instruction)                     {}
func (i instructionImpl) operands() []Value                                  { return nil }
func (i instructionImpl) calledValue() Value                                 { return Value{} }
func (i instructionImpl) icmpPredicate() string                              { return "" }
func (bb basicBlockImpl) asValue() Value                                     { return Value{} }
func newBuilderAtImpl(Instruction) builderImpl                               { return builderImpl{} }
func newBuilderAtEndImpl(BasicBlock) builderImpl                             { return builderImpl{} }
func (b builderImpl) createCall(Function, []Value) Instruction               { return Instruction{} }
func (b builderImpl) createAdd(Value, Value, string) Instruction             { return Instruction{} }
func (b builderImpl) createXor(Value, Value, string) Instruction             { return Instruction{} }
func (b builderImpl) createSub(Value, Value, string) Instruction             { return Instruction{} }
func (b builderImpl) createICmpEq(Value, Value, string) Instruction          { return Instruction{} }
func (b builderImpl) createBr(BasicBlock) Instruction                        { return Instruction{} }
func (b builderImpl) createCondBr(Value, BasicBlock, BasicBlock) Instruction { return Instruction{} }
func (b builderImpl) createRetVoid() Instruction                             { return Instruction{} }
func (b builderImpl) createBitCast(Value, ValueType, string) Instruction     { return Instruction{} }
func (b builderImpl) dispose()                                               {}
func (b builderImpl) setInsertPointAtEnd(BasicBlock)                         {}
func (b builderImpl) createAlloca(ValueType, string) Value                   { return Value{} }
func (b builderImpl) createLoad(ValueType, Value, string) Value              { return Value{} }
func (b builderImpl) createStore(Value, Value) Instruction                   { return Instruction{} }
func (b builderImpl) createSwitch(Value, BasicBlock, int) SwitchInst         { return SwitchInst{} }

type switchInstImpl struct{}

func (s switchInstImpl) addCase(Value, BasicBlock) {}

func (i instructionImpl) asValue() valueImpl   { return valueImpl{} }
func (v valueImpl) isConstInt() (bool, uint64) { return false, 0 }
func (v valueImpl) typ() ValueType             { return ValueType{} }
func (v valueImpl) name() string               { return "" }
func (v valueImpl) refID() uintptr             { return 0 }
func constIntImpl(int64, uint) Value           { return Value{} }
func constBoolImpl(bool) Value                 { return Value{} }
func (m moduleImpl) ensureFunction(string, ValueType, []ValueType) Function {
	return Function{}
}
func intTypeImpl(uint) ValueType                 { return ValueType{} }
func voidTypeImpl() ValueType                    { return ValueType{} }
func pointerTypeImpl(ValueType) ValueType        { return ValueType{} }
func (v valueTypeImpl) isZero() bool             { return true }
func (v valueTypeImpl) equal(valueTypeImpl) bool { return true }
func (v valueTypeImpl) intWidth() (int, bool)    { return 0, false }
