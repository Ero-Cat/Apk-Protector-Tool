package vmp

import (
	"bytes"
	"encoding/binary"
)

// BytecodeBuffer 提供字节码编码工具
type BytecodeBuffer struct {
	buf    *bytes.Buffer
	mapper *OpcodeMapper
}

// NewBytecodeBuffer 创建字节码缓冲区
func NewBytecodeBuffer(mapper *OpcodeMapper) *BytecodeBuffer {
	return &BytecodeBuffer{
		buf:    bytes.NewBuffer(nil),
		mapper: mapper,
	}
}

// Emit 写入单个操作码
func (b *BytecodeBuffer) Emit(op Opcode) {
	encoded := b.mapper.Encode(op)
	b.buf.WriteByte(byte(encoded))
}

// EmitRaw 写入原始字节（不进行 opcode 映射）
func (b *BytecodeBuffer) EmitRaw(data ...byte) {
	b.buf.Write(data)
}

// EmitU8 写入 1 字节无符号整数
func (b *BytecodeBuffer) EmitU8(v uint8) {
	b.buf.WriteByte(v)
}

// EmitU16 写入 2 字节无符号整数（小端序）
func (b *BytecodeBuffer) EmitU16(v uint16) {
	var data [2]byte
	binary.LittleEndian.PutUint16(data[:], v)
	b.buf.Write(data[:])
}

// EmitU32 写入 4 字节无符号整数（小端序）
func (b *BytecodeBuffer) EmitU32(v uint32) {
	var data [4]byte
	binary.LittleEndian.PutUint32(data[:], v)
	b.buf.Write(data[:])
}

// EmitI32 写入 4 字节有符号整数（小端序）
func (b *BytecodeBuffer) EmitI32(v int32) {
	b.EmitU32(uint32(v))
}

// EmitI64 写入 8 字节有符号整数（小端序）
func (b *BytecodeBuffer) EmitI64(v int64) {
	var data [8]byte
	binary.LittleEndian.PutUint64(data[:], uint64(v))
	b.buf.Write(data[:])
}

// Len 返回当前缓冲区长度
func (b *BytecodeBuffer) Len() int {
	return b.buf.Len()
}

// Bytes 返回字节码数据
func (b *BytecodeBuffer) Bytes() []byte {
	return b.buf.Bytes()
}

// PatchU16 在指定偏移处修补 2 字节值
func (b *BytecodeBuffer) PatchU16(offset int, v uint16) {
	data := b.buf.Bytes()
	if offset+2 <= len(data) {
		binary.LittleEndian.PutUint16(data[offset:], v)
	}
}

// PatchI16 在指定偏移处修补 2 字节有符号值
func (b *BytecodeBuffer) PatchI16(offset int, v int16) {
	b.PatchU16(offset, uint16(v))
}

// Backpatch 表示待回填的跳转
type Backpatch struct {
	Offset    int    // 需要修补的偏移位置
	Target    string // 目标标签名
	Relative  bool   // 是否为相对偏移
	FromAfter int    // 从指令后计算偏移（用于相对跳转）
}

// LabelResolver 管理标签与偏移的映射
type LabelResolver struct {
	labels    map[string]int // 标签名 -> 偏移
	backpatch []Backpatch    // 待回填列表
}

// NewLabelResolver 创建标签解析器
func NewLabelResolver() *LabelResolver {
	return &LabelResolver{
		labels:    make(map[string]int),
		backpatch: nil,
	}
}

// DefineLabel 定义标签位置
func (r *LabelResolver) DefineLabel(name string, offset int) {
	r.labels[name] = offset
}

// AddBackpatch 添加待回填项
func (r *LabelResolver) AddBackpatch(bp Backpatch) {
	r.backpatch = append(r.backpatch, bp)
}

// Resolve 解析所有待回填跳转
func (r *LabelResolver) Resolve(buf *BytecodeBuffer) error {
	for _, bp := range r.backpatch {
		targetOffset, ok := r.labels[bp.Target]
		if !ok {
			// 未知标签，使用 0
			targetOffset = 0
		}

		var value int16
		if bp.Relative {
			// 相对跳转：目标 - (当前位置 + 指令长度)
			value = int16(targetOffset - bp.FromAfter)
		} else {
			// 绝对跳转
			value = int16(targetOffset)
		}

		buf.PatchI16(bp.Offset, value)
	}
	return nil
}

// GetOffset 获取标签偏移
func (r *LabelResolver) GetOffset(name string) (int, bool) {
	off, ok := r.labels[name]
	return off, ok
}
