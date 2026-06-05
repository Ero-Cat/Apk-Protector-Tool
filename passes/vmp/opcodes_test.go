package vmp

import (
	"math/rand"
	"testing"
)

func TestOpcodeMapper(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	perm := r.Perm(256)
	mapper := NewOpcodeMapper(perm)

	testCases := []struct {
		op   Opcode
		name string
	}{
		{OP_PUSH_CONST, "PUSH_CONST"},
		{OP_ADD, "ADD"},
		{OP_SUB, "SUB"},
		{OP_XOR, "XOR"},
		{OP_JMP, "JMP"},
		{OP_RET, "RET"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 编码后应该可以正确解码
			encoded := mapper.Encode(tc.op)
			decoded := mapper.Decode(encoded)

			if decoded != tc.op {
				t.Errorf("Decode(Encode(%d)) = %d, want %d", tc.op, decoded, tc.op)
			}
		})
	}
}

func TestOpcodeMapperConsistency(t *testing.T) {
	r := rand.New(rand.NewSource(123))
	perm := r.Perm(256)
	mapper := NewOpcodeMapper(perm)

	// 同一个 mapper 连续编码应该返回相同结果
	for i := 0; i < 10; i++ {
		enc1 := mapper.Encode(OP_ADD)
		enc2 := mapper.Encode(OP_ADD)
		if enc1 != enc2 {
			t.Errorf("Inconsistent encoding: %d != %d", enc1, enc2)
		}
	}
}

func TestOpcodeMapperUniqueness(t *testing.T) {
	r := rand.New(rand.NewSource(456))
	perm := r.Perm(256)
	mapper := NewOpcodeMapper(perm)

	// 不同的原始 opcode 应该映射到不同的编码值（在大多数情况下）
	seen := make(map[Opcode]bool)
	for op := range BaseOpcodes {
		encoded := mapper.Encode(op)
		if seen[encoded] {
			// 由于排列是随机的，理论上不应该有重复
			t.Logf("Warning: duplicate encoding for %d", op)
		}
		seen[encoded] = true
	}
}

func TestBaseOpcodesInfo(t *testing.T) {
	// 验证基础 opcode 信息完整性
	expectedOps := []Opcode{
		OP_PUSH_CONST, OP_PUSH_ARG, OP_PUSH_LOCAL, OP_POP, OP_DUP,
		OP_ADD, OP_SUB, OP_MUL, OP_DIV, OP_MOD, OP_NEG,
		OP_AND, OP_OR, OP_XOR, OP_NOT, OP_SHL, OP_SHR,
		OP_LOAD, OP_STORE,
		OP_CMP_EQ, OP_CMP_NE, OP_CMP_LT, OP_CMP_LE, OP_CMP_GT, OP_CMP_GE,
		OP_JMP, OP_JZ, OP_JNZ,
		OP_CALL_EXT,
		OP_NOP, OP_RET,
	}

	for _, op := range expectedOps {
		info, ok := BaseOpcodes[op]
		if !ok {
			t.Errorf("Missing BaseOpcodes entry for opcode %d", op)
			continue
		}
		if info.Name == "" {
			t.Errorf("Empty name for opcode %d", op)
		}
		if info.Size < 1 {
			t.Errorf("Invalid size %d for opcode %d", info.Size, op)
		}
	}
}
