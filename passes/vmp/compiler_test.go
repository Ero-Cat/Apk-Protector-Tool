package vmp

import (
	"strings"
	"testing"

	"github.com/Ero-Cat/Apk-Protector-Tool/llvmwrap"
)

// TestIcmpPredicateMapping 覆盖全部 10 个 LLVM 整数比较谓词（P1.4 验收标准）。
func TestIcmpPredicateMapping(t *testing.T) {
	tests := map[string]Opcode{
		"eq": OP_CMP_EQ, "ne": OP_CMP_NE,
		"sgt": OP_CMP_GT, "sge": OP_CMP_GE,
		"slt": OP_CMP_LT, "sle": OP_CMP_LE,
		"ugt": OP_CMP_UGT, "uge": OP_CMP_UGE,
		"ult": OP_CMP_ULT, "ule": OP_CMP_ULE,
	}
	for predicate, want := range tests {
		got, err := icmpOpcode(predicate)
		if err != nil {
			t.Fatalf("icmpOpcode(%q): %v", predicate, err)
		}
		if got != want {
			t.Errorf("icmpOpcode(%q) = 0x%02x, want 0x%02x", predicate, got, want)
		}
	}
	// 空谓词回退 EQ（兼容无法报告谓词的封装层）。
	if got, _ := icmpOpcode(""); got != OP_CMP_EQ {
		t.Errorf("empty predicate should fall back to EQ, got 0x%02x", got)
	}
	if _, err := icmpOpcode("bogus"); err == nil {
		t.Error("unknown predicate should error")
	}
}

// TestResolveRejectsUnknownLabels 验证未知标签回填报错而非静默跳 0（P1.3 验收标准）。
func TestResolveRejectsUnknownLabels(t *testing.T) {
	res := NewLabelResolver()
	buf := NewBytecodeBuffer(NewOpcodeMapper(nil))
	buf.Emit(OP_JMP)
	res.AddBackpatch(Backpatch{Offset: 1, Target: "nowhere", Relative: true, FromAfter: 3})
	err := res.Resolve(buf)
	if err == nil {
		t.Fatal("Resolve should reject unknown labels")
	}
	if !strings.Contains(err.Error(), "nowhere") {
		t.Fatalf("error should name the missing label, got: %v", err)
	}
}

// TestBlockLabel 验证匿名块获得稳定的按位置标签。零值 BasicBlock 只在
// mock 构建下安全（native 下会解引用空指针），具名块路径由 LLVM 集成
// 测试覆盖（见 ROADMAP P4.2）。
func TestBlockLabel(t *testing.T) {
	if llvmwrap.HasNative() {
		t.Skip("zero-value BasicBlock is mock-only; named-block path covered by llvm-tagged tests")
	}
	if got := blockLabel(llvmwrap.BasicBlock{}, 3); got != "bb3" {
		t.Fatalf("unnamed block label = %q, want bb3", got)
	}
}
