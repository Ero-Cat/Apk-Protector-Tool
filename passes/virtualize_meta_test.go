package passes

import (
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/Ero-Cat/Apk-Protector-Tool/config"
	"github.com/Ero-Cat/Apk-Protector-Tool/passes/vmp"
	"github.com/Ero-Cat/Apk-Protector-Tool/report"
)

// TestMetadataJSONExportsDecoderEssentials 验证元数据包含运行时解码/解密
// 所需的全部字段：opcode 表、真实长度与密钥片段（P1.1/P1.6 验收标准）。
func TestMetadataJSONExportsDecoderEssentials(t *testing.T) {
	p := &VirtualizePass{
		Cfg:    &config.Config{},
		Rand:   rand.New(rand.NewSource(42)),
		Report: report.New(),
	}

	mapper := vmp.NewOpcodeMapper([]int{5, 7, 11})
	result := &vmp.CompileResult{
		Bytecode:   make([]byte, 33),
		Mapper:     mapper,
		LocalCount: 4,
		ParamCount: 2,
	}

	raw := p.metadataJSON("target_fn", "vm_a", 0xC7, result)

	var meta map[string]any
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatalf("metadata is not valid JSON: %v\n%s", err, raw)
	}

	if meta["bytecode_len"] != float64(33) {
		t.Fatalf("bytecode_len = %v, want 33", meta["bytecode_len"])
	}
	if meta["encrypted"] != true {
		t.Fatalf("encrypted = %v, want true", meta["encrypted"])
	}
	if meta["key_pad"] != float64(0xC7) {
		t.Fatalf("key_pad = %v, want 199", meta["key_pad"])
	}
	if meta["param_count"] != float64(2) {
		t.Fatalf("param_count = %v, want 2", meta["param_count"])
	}

	opcodes, ok := meta["opcodes"].(map[string]any)
	if !ok {
		t.Fatalf("opcodes missing or not an object: %s", raw)
	}
	if len(opcodes) != len(vmp.BaseOpcodes) {
		t.Fatalf("opcodes has %d entries, want %d", len(opcodes), len(vmp.BaseOpcodes))
	}
	// 随机化映射必须在元数据里可还原：每个标准助记符都有随机化字节。
	for _, info := range vmp.BaseOpcodes {
		if _, ok := opcodes[info.Name]; !ok {
			t.Errorf("opcodes missing mnemonic %s", info.Name)
		}
	}
	// 映射值与编译器实际编码一致。
	if want := float64(mapper.Encode(vmp.OP_ADD)); opcodes["ADD"] != want {
		t.Errorf("opcodes[ADD] = %v, want %v", opcodes["ADD"], want)
	}

	// ext_funcs 已随 call 编译脚手架移除（P1.5 后续修正）：字段必须缺席。
	if _, present := meta["ext_funcs"]; present {
		t.Fatalf("ext_funcs must be absent, got %v", meta["ext_funcs"])
	}
}

// TestEncryptBytecodeKeySplit 验证密钥拆分模型：static ^ pad 可还原密文。
func TestEncryptBytecodeKeySplit(t *testing.T) {
	cfg := &config.Config{}
	cfg.VMP.StaticKey = "abcdefgh"
	p := &VirtualizePass{Cfg: cfg, Rand: rand.New(rand.NewSource(7)), Report: report.New()}

	plain := []byte("some very secret bytecode payload \x00\x01\x02")
	enc, pad := p.encryptBytecode(plain)

	key := p.staticKeyByte() ^ pad
	for i, b := range enc {
		if b^key != plain[i] {
			t.Fatalf("roundtrip failed at %d: %v %v", i, b^key, plain[i])
		}
	}
}

// TestStaticKeyByteDefault 验证缺省静态片段与运行时 g_static_key 默认值一致。
func TestStaticKeyByteDefault(t *testing.T) {
	p := &VirtualizePass{Cfg: &config.Config{}, Rand: rand.New(rand.NewSource(1)), Report: report.New()}
	if got := p.staticKeyByte(); got != 0x5a {
		t.Fatalf("default static key = 0x%02x, want 0x5A (must match runtime g_static_key)", got)
	}
}
