package passes

import (
	"math/rand"

	"github.com/Ero-Cat/Apk-Protector-Tool/llvmwrap"
)

// keySlots 为单个函数维护"位宽 -> 密钥槽"的惰性分配。
// IRBuilder 会对全常量操作数的二元运算做常数折叠：直接 xor(c, k) 会折回
// 常量、包裹静默消失。把密钥 store 进栈槽再 load 出来，load 结果不是常量，
// 恒等链才真正落到 IR 里。
type keySlots struct {
	entry llvmwrap.Instruction // 函数入口首指令，槽插在它之前
	slots map[uint]llvmwrap.Value
}

func newKeySlots(entry llvmwrap.Instruction) *keySlots {
	return &keySlots{entry: entry, slots: map[uint]llvmwrap.Value{}}
}

func (ks *keySlots) slotFor(width uint) llvmwrap.Value {
	if s, ok := ks.slots[width]; ok {
		return s
	}
	b := llvmwrap.NewBuilderAt(ks.entry)
	s := b.CreateAlloca(llvmwrap.IntType(width), "gp.ks")
	b.Dispose()
	ks.slots[width] = s
	return s
}

// nonZeroKey 采样一个非零且落在 width 位内的密钥（0 会退化成可见的恒等）。
func nonZeroKey(r *rand.Rand, width int) llvmwrap.Value {
	var key int64 = 1
	switch {
	case width >= 8:
		key = int64(r.Intn(255)) + 1
	case width > 1:
		key = int64(r.Intn((1<<width)-2)) + 1
	}
	return llvmwrap.ConstInt(key, uint(width))
}

// wrapOperand 把 inst 的第 idx 个整型操作数替换为运行时可恢复的恒等表达式，
// 形态随机二选一：(x^k)^k 或 (x−k)+k（builder 不带 nsw 标记，无带符号溢出 UB）。
// 密钥经栈槽中转避开常数折叠；操作数本身是常量或 SSA 值均可。
func wrapOperand(r *rand.Rand, ks *keySlots, inst llvmwrap.Instruction, idx int, op llvmwrap.Value, width uint) {
	k := nonZeroKey(r, int(width))
	slot := ks.slotFor(width)
	b := llvmwrap.NewBuilderAt(inst)
	b.CreateStore(k, slot)
	kv := b.CreateLoad(llvmwrap.IntType(width), slot, "gp.kv")
	var replacement llvmwrap.Value
	if r.Intn(2) == 0 {
		x1 := b.CreateXor(op, kv.AsValue(), "gp.w.x1")
		x2 := b.CreateXor(x1.AsValue(), kv.AsValue(), "gp.w.x2")
		replacement = x2.AsValue()
	} else {
		s1 := b.CreateSub(op, kv.AsValue(), "gp.w.s1")
		a2 := b.CreateAdd(s1.AsValue(), kv.AsValue(), "gp.w.a2")
		replacement = a2.AsValue()
	}
	b.Dispose()
	inst.SetOperand(idx, replacement)
}
