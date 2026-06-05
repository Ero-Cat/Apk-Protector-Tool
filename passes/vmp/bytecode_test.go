package vmp

import (
	"math/rand"
	"testing"
)

func TestBytecodeBufferEmit(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	mapper := NewOpcodeMapper(r.Perm(256))
	buf := NewBytecodeBuffer(mapper)

	// Emit 一个操作码
	buf.Emit(OP_ADD)
	if buf.Len() != 1 {
		t.Errorf("Len() = %d, want 1", buf.Len())
	}

	// Emit 另一个操作码
	buf.Emit(OP_SUB)
	if buf.Len() != 2 {
		t.Errorf("Len() = %d, want 2", buf.Len())
	}
}

func TestBytecodeBufferEmitValues(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	mapper := NewOpcodeMapper(r.Perm(256))
	buf := NewBytecodeBuffer(mapper)

	// Emit U8
	buf.EmitU8(0x42)
	if buf.Len() != 1 {
		t.Errorf("Len() = %d, want 1", buf.Len())
	}

	// Emit U16 (小端序)
	buf.EmitU16(0x1234)
	if buf.Len() != 3 {
		t.Errorf("Len() = %d, want 3", buf.Len())
	}
	data := buf.Bytes()
	if data[1] != 0x34 || data[2] != 0x12 {
		t.Errorf("U16 encoding = [%02x, %02x], want [34, 12]", data[1], data[2])
	}

	// Emit U32 (小端序)
	buf.EmitU32(0xDEADBEEF)
	if buf.Len() != 7 {
		t.Errorf("Len() = %d, want 7", buf.Len())
	}
}

func TestBytecodeBufferPatch(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	mapper := NewOpcodeMapper(r.Perm(256))
	buf := NewBytecodeBuffer(mapper)

	// 先写入占位
	buf.EmitU16(0x0000)
	patchOffset := 0

	// 继续写入其他内容
	buf.EmitU8(0xFF)

	// 回填
	buf.PatchU16(patchOffset, 0xABCD)

	data := buf.Bytes()
	if data[0] != 0xCD || data[1] != 0xAB {
		t.Errorf("Patched value = [%02x, %02x], want [CD, AB]", data[0], data[1])
	}
	if data[2] != 0xFF {
		t.Error("Patch corrupted other data")
	}
}

func TestLabelResolver(t *testing.T) {
	resolver := NewLabelResolver()

	// 定义标签
	resolver.DefineLabel("start", 0)
	resolver.DefineLabel("loop", 10)
	resolver.DefineLabel("end", 50)

	// 查询标签
	if off, ok := resolver.GetOffset("start"); !ok || off != 0 {
		t.Errorf("GetOffset(start) = (%d, %v), want (0, true)", off, ok)
	}
	if off, ok := resolver.GetOffset("loop"); !ok || off != 10 {
		t.Errorf("GetOffset(loop) = (%d, %v), want (10, true)", off, ok)
	}
	if off, ok := resolver.GetOffset("unknown"); ok {
		t.Errorf("GetOffset(unknown) = (%d, true), want (_, false)", off)
	}
}

func TestLabelResolverBackpatch(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	mapper := NewOpcodeMapper(r.Perm(256))
	buf := NewBytecodeBuffer(mapper)
	resolver := NewLabelResolver()

	// 模拟: JMP 指令（opcode + 2B offset）
	buf.Emit(OP_JMP)
	patchOffset := buf.Len()
	buf.EmitU16(0) // 占位

	// 添加回填项
	resolver.AddBackpatch(Backpatch{
		Offset:    patchOffset,
		Target:    "target",
		Relative:  true,
		FromAfter: buf.Len(),
	})

	// 定义目标标签（在当前位置后 20 字节）
	resolver.DefineLabel("target", buf.Len()+20)

	// 解析
	err := resolver.Resolve(buf)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// 检查回填值（相对偏移应该是 20）
	data := buf.Bytes()
	patchedValue := int16(data[patchOffset]) | (int16(data[patchOffset+1]) << 8)
	if patchedValue != 20 {
		t.Errorf("Backpatch result = %d, want 20", patchedValue)
	}
}
