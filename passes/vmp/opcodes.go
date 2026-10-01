package vmp

// VM 指令集定义与随机化逻辑

// Opcode 定义 VM 字节码操作码
type Opcode byte

// 基础指令集（编译期固定语义，运行时根据 opcode 映射解释）
const (
	// 栈操作
	OP_PUSH_CONST Opcode = 0x01 // 压入 4 字节常量
	OP_PUSH_ARG   Opcode = 0x02 // 压入参数槽
	OP_PUSH_LOCAL Opcode = 0x03 // 压入局部变量槽
	OP_POP        Opcode = 0x04 // 弹出栈顶
	OP_DUP        Opcode = 0x05 // 复制栈顶

	// 算术运算（操作数从栈顶取，结果压回）
	OP_ADD Opcode = 0x10
	OP_SUB Opcode = 0x11
	OP_MUL Opcode = 0x12
	OP_DIV Opcode = 0x13
	OP_MOD Opcode = 0x14
	OP_NEG Opcode = 0x15

	// 位运算
	OP_AND Opcode = 0x20
	OP_OR  Opcode = 0x21
	OP_XOR Opcode = 0x22
	OP_NOT Opcode = 0x23
	OP_SHL Opcode = 0x24
	OP_SHR Opcode = 0x25

	// 内存操作
	OP_LOAD  Opcode = 0x30 // 从地址加载
	OP_STORE Opcode = 0x31 // 存储到地址

	// 比较（结果为 0 或 1）
	OP_CMP_EQ  Opcode = 0x40
	OP_CMP_NE  Opcode = 0x41
	OP_CMP_LT  Opcode = 0x42
	OP_CMP_LE  Opcode = 0x43
	OP_CMP_GT  Opcode = 0x44
	OP_CMP_GE  Opcode = 0x45
	OP_CMP_ULT Opcode = 0x46 // 无符号小于
	OP_CMP_UGT Opcode = 0x47 // 无符号大于
	OP_CMP_ULE Opcode = 0x48 // 无符号小于等于
	OP_CMP_UGE Opcode = 0x49 // 无符号大于等于

	// 控制流
	OP_JMP Opcode = 0x50 // 无条件跳转（2 字节偏移）
	OP_JZ  Opcode = 0x51 // 栈顶为 0 则跳转
	OP_JNZ Opcode = 0x52 // 栈顶非 0 则跳转

	// 调用
	OP_CALL_EXT Opcode = 0x60 // 外部函数调用（2 字节函数 ID + 1 字节参数数）

	// 控制
	OP_NOP Opcode = 0xFE
	OP_RET Opcode = 0xFF
)

// OpcodeInfo 描述指令元数据
type OpcodeInfo struct {
	Name     string // 助记符
	Size     int    // 指令总长度（含操作码）
	Operands int    // 操作数数目
}

// BaseOpcodes 定义标准指令信息
var BaseOpcodes = map[Opcode]OpcodeInfo{
	OP_PUSH_CONST: {"PUSH_CONST", 5, 1}, // op + 4B value
	OP_PUSH_ARG:   {"PUSH_ARG", 2, 1},   // op + 1B index
	OP_PUSH_LOCAL: {"PUSH_LOCAL", 2, 1},
	OP_POP:        {"POP", 1, 0},
	OP_DUP:        {"DUP", 1, 0},

	OP_ADD: {"ADD", 1, 0},
	OP_SUB: {"SUB", 1, 0},
	OP_MUL: {"MUL", 1, 0},
	OP_DIV: {"DIV", 1, 0},
	OP_MOD: {"MOD", 1, 0},
	OP_NEG: {"NEG", 1, 0},

	OP_AND: {"AND", 1, 0},
	OP_OR:  {"OR", 1, 0},
	OP_XOR: {"XOR", 1, 0},
	OP_NOT: {"NOT", 1, 0},
	OP_SHL: {"SHL", 1, 0},
	OP_SHR: {"SHR", 1, 0},

	OP_LOAD:  {"LOAD", 1, 0},
	OP_STORE: {"STORE", 1, 0},

	OP_CMP_EQ:  {"CMP_EQ", 1, 0},
	OP_CMP_NE:  {"CMP_NE", 1, 0},
	OP_CMP_LT:  {"CMP_LT", 1, 0},
	OP_CMP_LE:  {"CMP_LE", 1, 0},
	OP_CMP_GT:  {"CMP_GT", 1, 0},
	OP_CMP_GE:  {"CMP_GE", 1, 0},
	OP_CMP_ULT: {"CMP_ULT", 1, 0},
	OP_CMP_UGT: {"CMP_UGT", 1, 0},
	OP_CMP_ULE: {"CMP_ULE", 1, 0},
	OP_CMP_UGE: {"CMP_UGE", 1, 0},

	OP_JMP: {"JMP", 3, 1}, // op + 2B offset
	OP_JZ:  {"JZ", 3, 1},
	OP_JNZ: {"JNZ", 3, 1},

	OP_CALL_EXT: {"CALL_EXT", 4, 2}, // op + 2B fn_id + 1B argc

	OP_NOP: {"NOP", 1, 0},
	OP_RET: {"RET", 1, 0},
}

// OpcodeMapper 管理 opcode 随机化映射
type OpcodeMapper struct {
	// 标准 -> 随机化
	Forward map[Opcode]Opcode
	// 随机化 -> 标准
	Reverse map[Opcode]Opcode
}

// NewOpcodeMapper 创建随机化映射
func NewOpcodeMapper(perm []int) *OpcodeMapper {
	// 获取所有基础 opcode
	opcodes := make([]Opcode, 0, len(BaseOpcodes))
	for op := range BaseOpcodes {
		opcodes = append(opcodes, op)
	}

	m := &OpcodeMapper{
		Forward: make(map[Opcode]Opcode),
		Reverse: make(map[Opcode]Opcode),
	}

	// 应用排列生成随机映射
	for i, op := range opcodes {
		if i < len(perm) {
			randomized := Opcode(perm[i])
			m.Forward[op] = randomized
			m.Reverse[randomized] = op
		} else {
			// 如果排列不够，使用原始值
			m.Forward[op] = op
			m.Reverse[op] = op
		}
	}

	return m
}

// Encode 将标准 opcode 转换为随机化 opcode
func (m *OpcodeMapper) Encode(op Opcode) Opcode {
	if mapped, ok := m.Forward[op]; ok {
		return mapped
	}
	return op
}

// Decode 将随机化 opcode 转换回标准 opcode
func (m *OpcodeMapper) Decode(op Opcode) Opcode {
	if mapped, ok := m.Reverse[op]; ok {
		return mapped
	}
	return op
}
