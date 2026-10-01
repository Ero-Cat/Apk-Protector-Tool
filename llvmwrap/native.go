//go:build llvm

package llvmwrap

/*
#cgo !windows pkg-config: llvm
#cgo windows LDFLAGS: -lLLVM
#include <llvm-c/Core.h>
#include <llvm-c/BitReader.h>
#include <llvm-c/BitWriter.h>
#include <llvm-c/Analysis.h>
#include <llvm-c/IRReader.h>
#include <stdlib.h>
#include <string.h>
*/
import "C"
import (
	"errors"
	"strings"
	"unsafe"
)

type moduleImpl struct{ ref C.LLVMModuleRef }
type functionImpl struct{ ref C.LLVMValueRef }
type basicBlockImpl struct{ ref C.LLVMBasicBlockRef }
type instructionImpl struct{ ref C.LLVMValueRef }
type builderImpl struct{ ref C.LLVMBuilderRef }
type valueImpl struct{ ref C.LLVMValueRef }
type valueTypeImpl struct{ ref C.LLVMTypeRef }

func hasNativeImpl() bool { return true }

func cstring(s string) *C.char { return C.CString(s) }

func parseBitcodeImpl(path string) (*Module, error) {
	cpath := cstring(path)
	defer C.free(unsafe.Pointer(cpath))

	var buf C.LLVMMemoryBufferRef
	var msg *C.char
	if C.LLVMCreateMemoryBufferWithContentsOfFile(cpath, &buf, &msg) != 0 {
		defer C.LLVMDisposeMessage(msg)
		return nil, errors.New(C.GoString(msg))
	}

	var mod C.LLVMModuleRef
	if strings.HasSuffix(path, ".ll") {
		// LLVMParseIRInContext takes ownership of the memory buffer on both
		// success and failure, so it must not be disposed on this path.
		if C.LLVMParseIRInContext(C.LLVMGetGlobalContext(), buf, &mod, &msg) != 0 {
			defer C.LLVMDisposeMessage(msg)
			return nil, errors.New(C.GoString(msg))
		}
		return &Module{impl: moduleImpl{ref: mod}}, nil
	}
	defer C.LLVMDisposeMemoryBuffer(buf)
	if C.LLVMParseBitcode2(buf, &mod) != 0 {
		return nil, errors.New("failed to parse bitcode")
	}
	return &Module{impl: moduleImpl{ref: mod}}, nil
}

func (m moduleImpl) writeBitcode(path string) error {
	cpath := cstring(path)
	defer C.free(unsafe.Pointer(cpath))
	if C.LLVMWriteBitcodeToFile(m.ref, cpath) != 0 {
		return errors.New("LLVMWriteBitcodeToFile failed")
	}
	return nil
}

func (m moduleImpl) dispose() { C.LLVMDisposeModule(m.ref) }

func (m moduleImpl) string() string {
	cstr := C.LLVMPrintModuleToString(m.ref)
	defer C.LLVMDisposeMessage(cstr)
	return C.GoString(cstr)
}

func (m moduleImpl) functions() []Function {
	out := []Function{}
	for fn := C.LLVMGetFirstFunction(m.ref); fn != nil; fn = C.LLVMGetNextFunction(fn) {
		out = append(out, Function{impl: functionImpl{ref: fn}})
	}
	return out
}

// globals enumerates the module's global variables.
func (m moduleImpl) globals() []Value {
	out := []Value{}
	for g := C.LLVMGetFirstGlobal(m.ref); g != nil; g = C.LLVMGetNextGlobal(g) {
		out = append(out, Value{impl: valueImpl{ref: g}})
	}
	return out
}

// addGlobalBytes adds a private, WRITABLE i8-array global. Used for
// string-encryption ciphertext that the runtime decrypts in place — the
// writable linkage is what keeps in-place XOR legal.
func (m moduleImpl) addGlobalBytes(name string, data []byte) Value {
	cname := cstring(name)
	defer C.free(unsafe.Pointer(cname))
	cstr := (*C.char)(C.CBytes(data))
	defer C.free(unsafe.Pointer(cstr))
	// DontNullTerminate=1: the payload length is exactly len(data).
	constStr := C.LLVMConstString(cstr, C.uint(len(data)), C.LLVMBool(1))
	ty := C.LLVMArrayType(C.LLVMInt8Type(), C.uint(len(data)))
	gv := C.LLVMAddGlobal(m.ref, ty, cname)
	C.LLVMSetInitializer(gv, constStr)
	C.LLVMSetGlobalConstant(gv, 0)
	C.LLVMSetLinkage(gv, C.LLVMPrivateLinkage)
	return Value{impl: valueImpl{ref: gv}}
}

// emitStrRegionsTable emits the extern-visible `<name>` array plus
// `<name>_count` describing encrypted string regions for the C runtime.
// Layout: { ptr, i32, i8 } — must stay in sync with
// goprotect_str_region_t in runtime/include/goprotect.h.
// A null placeholder slot is emitted for the zero-region case so the runtime
// externs always resolve alongside the decrypt call the same pass inserts.
func (m moduleImpl) emitStrRegionsTable(name string, regions []StrRegion) {
	i8ptr := C.LLVMPointerType(C.LLVMInt8Type(), 0)
	elems := []C.LLVMTypeRef{i8ptr, C.LLVMInt32Type(), C.LLVMInt8Type()}
	structTy := C.LLVMStructType(&elems[0], 3, 0)

	n := len(regions)
	slotCount := n
	if slotCount == 0 {
		slotCount = 1
	}
	parts := make([]C.LLVMValueRef, 0, slotCount)
	if n == 0 {
		empty := []C.LLVMValueRef{
			C.LLVMConstPointerNull(i8ptr),
			C.LLVMConstInt(C.LLVMInt32Type(), 0, 0),
			C.LLVMConstInt(C.LLVMInt8Type(), 0, 0),
		}
		parts = append(parts, C.LLVMConstStruct(&empty[0], 3, 0))
	} else {
		for _, r := range regions {
			fields := []C.LLVMValueRef{
				r.Data.impl.ref, // opaque pointers: a global's address is a ptr
				C.LLVMConstInt(C.LLVMInt32Type(), C.ulonglong(uint32(r.Length)), 0),
				C.LLVMConstInt(C.LLVMInt8Type(), C.ulonglong(r.Key), 0),
			}
			parts = append(parts, C.LLVMConstStruct(&fields[0], 3, 0))
		}
	}
	arr := C.LLVMConstArray(structTy, &parts[0], C.uint(slotCount))

	cname := cstring(name)
	defer C.free(unsafe.Pointer(cname))
	gv := C.LLVMAddGlobal(m.ref, C.LLVMTypeOf(arr), cname)
	C.LLVMSetInitializer(gv, arr)

	countName := cstring(name + "_count")
	defer C.free(unsafe.Pointer(countName))
	cg := C.LLVMAddGlobal(m.ref, C.LLVMInt32Type(), countName)
	C.LLVMSetInitializer(cg, C.LLVMConstInt(C.LLVMInt32Type(), C.ulonglong(n), 0))
}

func (m moduleImpl) addGlobalString(name string, data []byte) Value {
	cname := cstring(name)
	defer C.free(unsafe.Pointer(cname))
	// Append a NUL terminator so byte-code buffers and JSON metadata can be
	// read as C strings by the runtime.
	payload := make([]byte, len(data)+1)
	copy(payload, data)
	cstr := (*C.char)(C.CBytes(payload))
	defer C.free(unsafe.Pointer(cstr))
	constStr := C.LLVMConstString(cstr, C.uint(len(payload)), C.LLVMBool(1))
	ty := C.LLVMArrayType(C.LLVMInt8Type(), C.uint(len(payload)))
	gv := C.LLVMAddGlobal(m.ref, ty, cname)
	C.LLVMSetInitializer(gv, constStr)
	C.LLVMSetGlobalConstant(gv, 0)
	C.LLVMSetLinkage(gv, C.LLVMPrivateLinkage)
	return Value{impl: valueImpl{ref: gv}}
}

func (f functionImpl) name() string {
	return C.GoString(C.LLVMGetValueName(f.ref))
}

func (f functionImpl) basicBlocks() []BasicBlock {
	out := []BasicBlock{}
	for bb := C.LLVMGetFirstBasicBlock(f.ref); bb != nil; bb = C.LLVMGetNextBasicBlock(bb) {
		out = append(out, BasicBlock{impl: basicBlockImpl{ref: bb}})
	}
	return out
}

func (f functionImpl) appendBasicBlock(name string) BasicBlock {
	cname := cstring(name)
	defer C.free(unsafe.Pointer(cname))
	bb := C.LLVMAppendBasicBlock(f.ref, cname)
	return BasicBlock{impl: basicBlockImpl{ref: bb}}
}

func (f functionImpl) typ() ValueType {
	return ValueType{impl: valueTypeImpl{ref: C.LLVMTypeOf(f.ref)}}
}

// functionTypeOf returns the FunctionType of a function value. Under opaque
// pointers (LLVM >= 15) LLVMTypeOf(function) is a plain ptr, and dereferencing
// its "element type" yields garbage that crashes builders — the C API exposes
// the real function type via LLVMGlobalGetValueType.
func functionTypeOf(fn C.LLVMValueRef) C.LLVMTypeRef {
	return C.LLVMGlobalGetValueType(fn)
}

func (f functionImpl) returnType() ValueType {
	ret := C.LLVMGetReturnType(functionTypeOf(f.ref))
	return ValueType{impl: valueTypeImpl{ref: ret}}
}

func (f functionImpl) paramCount() int {
	return int(C.LLVMCountParams(f.ref))
}

func (f functionImpl) param(i int) Value {
	return Value{impl: valueImpl{ref: C.LLVMGetParam(f.ref, C.uint(i))}}
}

func (f functionImpl) asValue() valueImpl { return valueImpl{ref: f.ref} }

func (bb basicBlockImpl) instructions() []Instruction {
	out := []Instruction{}
	for inst := C.LLVMGetFirstInstruction(bb.ref); inst != nil; inst = C.LLVMGetNextInstruction(inst) {
		out = append(out, Instruction{impl: instructionImpl{ref: inst}})
	}
	return out
}

func (bb basicBlockImpl) moveBefore(target BasicBlock) {
	C.LLVMMoveBasicBlockBefore(bb.ref, target.impl.ref)
}

// successors walks the terminator's successor list, which covers br, condbr
// and switch uniformly. The C API exposes successor accessors on the
// terminator value, not on the block ref.
func (bb basicBlockImpl) successors() []BasicBlock {
	term := C.LLVMGetBasicBlockTerminator(bb.ref)
	if term == nil {
		return nil
	}
	n := int(C.LLVMGetNumSuccessors(term))
	out := make([]BasicBlock, 0, n)
	for i := 0; i < n; i++ {
		succ := C.LLVMGetSuccessor(term, C.uint(i))
		if succ != nil {
			out = append(out, BasicBlock{impl: basicBlockImpl{ref: succ}})
		}
	}
	return out
}

// delete removes the (empty) basic block from its function. Callers must
// first erase every instruction — including terminators that reference it.
func (bb basicBlockImpl) delete() {
	C.LLVMDeleteBasicBlock(bb.ref)
}

func (bb basicBlockImpl) name() string {
	return C.GoString(C.LLVMGetBasicBlockName(bb.ref))
}

// opcodeNames maps LLVMOpcode enum values to textual opcode names. The C API
// has no LLVMGetOpcodeName accessor, so the mapping lives here and must track
// the LLVMOpcode enum in llvm-c/Core.h (values are stable by contract).
var opcodeNames = map[C.LLVMOpcode]string{
	C.LLVMRet:           "ret",
	C.LLVMUncondBr:      "br",
	C.LLVMCondBr:        "br",
	C.LLVMSwitch:        "switch",
	C.LLVMIndirectBr:    "indirectbr",
	C.LLVMInvoke:        "invoke",
	C.LLVMUnreachable:   "unreachable",
	C.LLVMCallBr:        "callbr",
	C.LLVMFNeg:          "fneg",
	C.LLVMCall:          "call",
	C.LLVMFence:         "fence",
	C.LLVMICmp:          "icmp",
	C.LLVMFCmp:          "fcmp",
	C.LLVMPHI:           "phi",
	C.LLVMSelect:        "select",
	C.LLVMFreeze:        "freeze",
	C.LLVMAlloca:        "alloca",
	C.LLVMLoad:          "load",
	C.LLVMStore:         "store",
	C.LLVMGetElementPtr: "getelementptr",
	C.LLVMTrunc:         "trunc",
	C.LLVMZExt:          "zext",
	C.LLVMSExt:          "sext",
	C.LLVMFPToUI:        "fptoui",
	C.LLVMFPToSI:        "fptosi",
	C.LLVMUIToFP:        "uitofp",
	C.LLVMSIToFP:        "sitofp",
	C.LLVMFPTrunc:       "fptrunc",
	C.LLVMFPExt:         "fpext",
	C.LLVMPtrToInt:      "ptrtoint",
	C.LLVMIntToPtr:      "inttoptr",
	C.LLVMBitCast:       "bitcast",
	C.LLVMAddrSpaceCast: "addrspacecast",
	C.LLVMAdd:           "add",
	C.LLVMFAdd:          "fadd",
	C.LLVMSub:           "sub",
	C.LLVMFSub:          "fsub",
	C.LLVMMul:           "mul",
	C.LLVMFMul:          "fmul",
	C.LLVMUDiv:          "udiv",
	C.LLVMSDiv:          "sdiv",
	C.LLVMFDiv:          "fdiv",
	C.LLVMURem:          "urem",
	C.LLVMSRem:          "srem",
	C.LLVMFRem:          "frem",
	C.LLVMShl:           "shl",
	C.LLVMLShr:          "lshr",
	C.LLVMAShr:          "ashr",
	C.LLVMAnd:           "and",
	C.LLVMOr:            "or",
	C.LLVMXor:           "xor",
}

func (i instructionImpl) opcode() string {
	if name, ok := opcodeNames[C.LLVMGetInstructionOpcode(i.ref)]; ok {
		return name
	}
	return "unknown"
}

func (i instructionImpl) replaceAllUsesWith(newInst Instruction) {
	C.LLVMReplaceAllUsesWith(i.ref, newInst.impl.ref)
}

func (i instructionImpl) operands() []Value {
	n := int(C.LLVMGetNumOperands(i.ref))
	out := make([]Value, 0, n)
	for idx := 0; idx < n; idx++ {
		op := C.LLVMGetOperand(i.ref, C.uint(idx))
		if op != nil {
			out = append(out, Value{impl: valueImpl{ref: op}})
		}
	}
	return out
}

func (i instructionImpl) calledValue() Value {
	return Value{impl: valueImpl{ref: C.LLVMGetCalledValue(i.ref)}}
}

func (i instructionImpl) setOperand(idx int, v Value) {
	C.LLVMSetOperand(i.ref, C.uint(idx), v.impl.ref)
}

func (i instructionImpl) eraseFromParent() {
	C.LLVMInstructionEraseFromParent(i.ref)
}

func (bb basicBlockImpl) terminator() Instruction {
	return Instruction{impl: instructionImpl{ref: C.LLVMGetBasicBlockTerminator(bb.ref)}}
}

func (m moduleImpl) verify() error {
	var msg *C.char
	// LLVMVerifyModule returns non-zero for invalid modules; the message
	// pointer must be provided (NULL makes failures indistinguishable).
	if rc := C.LLVMVerifyModule(m.ref, C.LLVMReturnStatusAction, &msg); rc != 0 {
		defer C.LLVMDisposeMessage(msg)
		if msg != nil {
			return errors.New(C.GoString(msg))
		}
		return errors.New("module failed verification")
	}
	if msg != nil {
		C.LLVMDisposeMessage(msg)
	}
	return nil
}

// intPredicateNames maps LLVMIntPredicate values to their textual form.
var intPredicateNames = map[C.LLVMIntPredicate]string{
	C.LLVMIntEQ:  "eq",
	C.LLVMIntNE:  "ne",
	C.LLVMIntUGT: "ugt",
	C.LLVMIntUGE: "uge",
	C.LLVMIntULT: "ult",
	C.LLVMIntULE: "ule",
	C.LLVMIntSGT: "sgt",
	C.LLVMIntSGE: "sge",
	C.LLVMIntSLT: "slt",
	C.LLVMIntSLE: "sle",
}

func (i instructionImpl) icmpPredicate() string {
	if C.LLVMGetInstructionOpcode(i.ref) != C.LLVMICmp {
		return ""
	}
	if name, ok := intPredicateNames[C.LLVMGetICmpPredicate(i.ref)]; ok {
		return name
	}
	return ""
}

func (bb basicBlockImpl) asValue() Value {
	return Value{impl: valueImpl{ref: C.LLVMBasicBlockAsValue(bb.ref)}}
}

func newBuilderAtImpl(instr Instruction) builderImpl {
	b := C.LLVMCreateBuilder()
	C.LLVMPositionBuilderBefore(b, instr.impl.ref)
	return builderImpl{ref: b}
}

func newBuilderAtEndImpl(bb BasicBlock) builderImpl {
	b := C.LLVMCreateBuilder()
	C.LLVMPositionBuilderAtEnd(b, bb.impl.ref)
	return builderImpl{ref: b}
}

func (b builderImpl) createCall(fn Function, args []Value) Instruction {
	argc := C.uint(len(args))
	var carg *C.LLVMValueRef
	if argc > 0 {
		tmp := make([]C.LLVMValueRef, len(args))
		for i, a := range args {
			tmp[i] = a.impl.ref
		}
		carg = &tmp[0]
	}
	cname := cstring("")
	defer C.free(unsafe.Pointer(cname))
	// Opaque pointers: LLVMTypeOf(fn) is a pointer, not the function type —
	// see functionTypeOf.
	call := C.LLVMBuildCall2(b.ref, functionTypeOf(fn.impl.ref), fn.impl.ref, carg, argc, cname)
	return Instruction{impl: instructionImpl{ref: call}}
}

func (b builderImpl) createAdd(lhs, rhs Value, name string) Instruction {
	cname := cstring(name)
	defer C.free(unsafe.Pointer(cname))
	inst := C.LLVMBuildAdd(b.ref, lhs.impl.ref, rhs.impl.ref, cname)
	return Instruction{impl: instructionImpl{ref: inst}}
}

func (b builderImpl) createXor(lhs, rhs Value, name string) Instruction {
	cname := cstring(name)
	defer C.free(unsafe.Pointer(cname))
	inst := C.LLVMBuildXor(b.ref, lhs.impl.ref, rhs.impl.ref, cname)
	return Instruction{impl: instructionImpl{ref: inst}}
}

func (b builderImpl) createSub(lhs, rhs Value, name string) Instruction {
	cname := cstring(name)
	defer C.free(unsafe.Pointer(cname))
	inst := C.LLVMBuildSub(b.ref, lhs.impl.ref, rhs.impl.ref, cname)
	return Instruction{impl: instructionImpl{ref: inst}}
}

func (b builderImpl) createICmpEq(lhs, rhs Value, name string) Instruction {
	cname := cstring(name)
	defer C.free(unsafe.Pointer(cname))
	inst := C.LLVMBuildICmp(b.ref, C.LLVMIntEQ, lhs.impl.ref, rhs.impl.ref, cname)
	return Instruction{impl: instructionImpl{ref: inst}}
}

func (b builderImpl) createBr(bb BasicBlock) Instruction {
	inst := C.LLVMBuildBr(b.ref, bb.impl.ref)
	return Instruction{impl: instructionImpl{ref: inst}}
}

func (b builderImpl) createCondBr(cond Value, t, f BasicBlock) Instruction {
	inst := C.LLVMBuildCondBr(b.ref, cond.impl.ref, t.impl.ref, f.impl.ref)
	return Instruction{impl: instructionImpl{ref: inst}}
}

func (b builderImpl) createRetVoid() Instruction {
	inst := C.LLVMBuildRetVoid(b.ref)
	return Instruction{impl: instructionImpl{ref: inst}}
}

func (b builderImpl) createRet(v Value) Instruction {
	inst := C.LLVMBuildRet(b.ref, v.impl.ref)
	return Instruction{impl: instructionImpl{ref: inst}}
}

func (b builderImpl) createBitCast(v Value, dst ValueType, name string) Instruction {
	cname := cstring(name)
	defer C.free(unsafe.Pointer(cname))
	inst := C.LLVMBuildBitCast(b.ref, v.impl.ref, dst.impl.ref, cname)
	return Instruction{impl: instructionImpl{ref: inst}}
}

func (b builderImpl) dispose() { C.LLVMDisposeBuilder(b.ref) }

func (b builderImpl) setInsertPointAtEnd(bb BasicBlock) {
	C.LLVMPositionBuilderAtEnd(b.ref, bb.impl.ref)
}

func (b builderImpl) createAlloca(ty ValueType, name string) Value {
	cname := cstring(name)
	defer C.free(unsafe.Pointer(cname))
	inst := C.LLVMBuildAlloca(b.ref, ty.impl.ref, cname)
	return Value{impl: valueImpl{ref: inst}}
}

func (b builderImpl) createLoad(ty ValueType, ptr Value, name string) Value {
	cname := cstring(name)
	defer C.free(unsafe.Pointer(cname))
	inst := C.LLVMBuildLoad2(b.ref, ty.impl.ref, ptr.impl.ref, cname)
	return Value{impl: valueImpl{ref: inst}}
}

func (b builderImpl) createStore(val Value, ptr Value) Instruction {
	inst := C.LLVMBuildStore(b.ref, val.impl.ref, ptr.impl.ref)
	return Instruction{impl: instructionImpl{ref: inst}}
}

type switchInstImpl struct{ ref C.LLVMValueRef }

func (b builderImpl) createSwitch(cond Value, defaultBB BasicBlock, numCases int) SwitchInst {
	inst := C.LLVMBuildSwitch(b.ref, cond.impl.ref, defaultBB.impl.ref, C.uint(numCases))
	return SwitchInst{impl: switchInstImpl{ref: inst}}
}

func (s switchInstImpl) addCase(val Value, dest BasicBlock) {
	C.LLVMAddCase(s.ref, val.impl.ref, dest.impl.ref)
}

func (i instructionImpl) asValue() valueImpl { return valueImpl{ref: i.ref} }

func (v valueImpl) isConstInt() (bool, uint64) {
	if C.LLVMIsAConstantInt(v.ref) == nil {
		return false, 0
	}
	return true, uint64(C.LLVMConstIntGetZExtValue(v.ref))
}

func (v valueImpl) name() string   { return C.GoString(C.LLVMGetValueName(v.ref)) }
func (v valueImpl) refID() uintptr { return uintptr(unsafe.Pointer(v.ref)) }

func (v valueImpl) typ() ValueType {
	return ValueType{impl: valueTypeImpl{ref: C.LLVMTypeOf(v.ref)}}
}

// users enumerates the values using this one (LLVM use chains).
func (v valueImpl) users() []Value {
	out := []Value{}
	for u := C.LLVMGetFirstUse(v.ref); u != nil; u = C.LLVMGetNextUse(u) {
		out = append(out, Value{impl: valueImpl{ref: C.LLVMGetUser(u)}})
	}
	return out
}

// isInstruction reports whether the value is an instruction (class-cast
// probe); constant expressions and other non-instruction users return false.
func (v valueImpl) isInstruction() bool {
	return C.LLVMIsAInstruction(v.ref) != nil
}

func (v valueImpl) isPrivateLinkage() bool {
	return C.LLVMGetLinkage(v.ref) == C.LLVMPrivateLinkage
}

func (v valueImpl) isGlobalConstant() bool {
	return C.LLVMIsGlobalConstant(v.ref) != 0
}

func (v valueImpl) initializer() Value {
	return Value{impl: valueImpl{ref: C.LLVMGetInitializer(v.ref)}}
}

// deleteGlobal destroys the global variable; it must have no remaining uses.
func (v valueImpl) deleteGlobal() {
	C.LLVMDeleteGlobal(v.ref)
}

// constantDataArrayBytes returns the exact byte contents (including any
// trailing NUL) of a ConstantDataArray of i8, e.g. a c"..." literal.
func (v valueImpl) constantDataArrayBytes() ([]byte, bool) {
	if C.LLVMIsAConstantDataArray(v.ref) == nil {
		return nil, false
	}
	ty := C.LLVMTypeOf(v.ref)
	if C.LLVMGetTypeKind(ty) != C.LLVMArrayTypeKind {
		return nil, false
	}
	elemTy := C.LLVMGetElementType(ty)
	if C.LLVMGetTypeKind(elemTy) != C.LLVMIntegerTypeKind || C.LLVMGetIntTypeWidth(elemTy) != 8 {
		return nil, false
	}
	n := int(C.LLVMGetArrayLength(ty))
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		el := C.LLVMGetAggregateElement(v.ref, C.uint(i))
		if el == nil {
			return nil, false
		}
		out[i] = byte(C.LLVMConstIntGetZExtValue(el))
	}
	return out, true
}

func (v valueImpl) asInstruction() Instruction {
	return Instruction{impl: instructionImpl{ref: v.ref}}
}

func constIntImpl(v int64, bits uint) Value {
	ty := C.LLVMIntType(C.unsigned(bits))
	val := C.LLVMConstInt(ty, C.ulonglong(v), 0)
	return Value{impl: valueImpl{ref: val}}
}

func constBoolImpl(v bool) Value {
	var n C.ulonglong
	if v {
		n = 1
	}
	val := C.LLVMConstInt(C.LLVMInt1Type(), n, 0)
	return Value{impl: valueImpl{ref: val}}
}

func (m moduleImpl) ensureFunction(name string, ret ValueType, params []ValueType) Function {
	cname := cstring(name)
	defer C.free(unsafe.Pointer(cname))
	existing := C.LLVMGetNamedFunction(m.ref, cname)
	if existing != nil {
		return Function{impl: functionImpl{ref: existing}}
	}
	var cparams *C.LLVMTypeRef
	if len(params) > 0 {
		arr := make([]C.LLVMTypeRef, len(params))
		for i, p := range params {
			arr[i] = p.impl.ref
		}
		cparams = &arr[0]
	}
	fnType := C.LLVMFunctionType(ret.impl.ref, cparams, C.uint(len(params)), C.int(0))
	fn := C.LLVMAddFunction(m.ref, cname, fnType)
	return Function{impl: functionImpl{ref: fn}}
}

func intTypeImpl(bits uint) ValueType {
	return ValueType{impl: valueTypeImpl{ref: C.LLVMIntType(C.unsigned(bits))}}
}
func voidTypeImpl() ValueType { return ValueType{impl: valueTypeImpl{ref: C.LLVMVoidType()}} }
func pointerTypeImpl(elem ValueType) ValueType {
	return ValueType{impl: valueTypeImpl{ref: C.LLVMPointerType(elem.impl.ref, 0)}}
}

func (v valueTypeImpl) isZero() bool               { return v.ref == nil }
func (v valueTypeImpl) equal(o valueTypeImpl) bool { return v.ref == o.ref }
func (v valueTypeImpl) intWidth() (int, bool) {
	if C.LLVMGetTypeKind(v.ref) != C.LLVMIntegerTypeKind {
		return 0, false
	}
	return int(C.LLVMGetIntTypeWidth(v.ref)), true
}
