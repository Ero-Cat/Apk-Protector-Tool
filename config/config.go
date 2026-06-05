package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 描述工具的输入输出、启用的 Pass 以及混淆强度。
type Config struct {
	Input  string `json:"input" yaml:"input"`
	Output string `json:"output" yaml:"output"`

	Functions struct {
		Allow []string `json:"allow" yaml:"allow"`
		Deny  []string `json:"deny" yaml:"deny"`
	} `json:"functions" yaml:"functions"`

	Passes struct {
		EntryExit        bool `json:"entry_exit" yaml:"entry_exit"`
		ConstSplit       bool `json:"const_split" yaml:"const_split"`
		CFFlatten        bool `json:"cf_flatten" yaml:"cf_flatten"`
		InstrSubstitute  bool `json:"instr_substitute" yaml:"instr_substitute"`
		ConstObfuscation bool `json:"const_obfuscation" yaml:"const_obfuscation"`
		Virtualization   bool `json:"virtualization" yaml:"virtualization"`
		SecurityHooks    bool `json:"security_hooks" yaml:"security_hooks"`
	} `json:"passes" yaml:"passes"`

	Obfuscation struct {
		Level               string `json:"level" yaml:"level"`
		SubstituteIntensity int    `json:"substitute_intensity" yaml:"substitute_intensity"`
		FlattenRatio        int    `json:"flatten_ratio" yaml:"flatten_ratio"` // percentage of eligible functions
		VirtualizeRatio     int    `json:"virtualize_ratio" yaml:"virtualize_ratio"`
	} `json:"obfuscation" yaml:"obfuscation"`

	Report struct {
		Path string `json:"path" yaml:"path"`
	} `json:"report" yaml:"report"`

	Debug struct {
		DumpCFG bool `json:"dump_cfg" yaml:"dump_cfg"`
	} `json:"debug" yaml:"debug"`

	VMP struct {
		// 是否为虚拟化层启用多 VM 随机化。
		EnableMultiVM bool `json:"enable_multi_vm" yaml:"enable_multi_vm"`
		// 每个 VM 的名称与 ISA 标签。
		VMs []VMClass `json:"vms" yaml:"vms"`
		// 按函数分类的保护等级：normal/sensitive/critical，可映射到不同 VM。
		Levels VMLevels `json:"levels" yaml:"levels"`
		// 静态密钥片段，用于字节码异或加密；可与运行时片段组合。
		StaticKey string `json:"static_key" yaml:"static_key"`
		// 运行时密钥提示（不会在编译期求值，仅写入元数据供运行时组合）。
		RuntimeKeyHint string `json:"runtime_key_hint" yaml:"runtime_key_hint"`
	} `json:"vmp" yaml:"vmp"`
}

// VMClass 定义单个 VM 的标识与 ISA 标签。
type VMClass struct {
	Name string `json:"name" yaml:"name"`
	ISA  string `json:"isa" yaml:"isa"`
}

// VMLevels 为不同保护等级指定 VM 名称。
type VMLevels struct {
	Normal    string `json:"normal" yaml:"normal"`
	Sensitive string `json:"sensitive" yaml:"sensitive"`
	Critical  string `json:"critical" yaml:"critical"`
	// 函数名单重写：某些函数强制绑定到指定 VM。
	FunctionVM    map[string]string `json:"function_vm" yaml:"function_vm"`
	SensitiveList []string          `json:"sensitive_list" yaml:"sensitive_list"`
	CriticalList  []string          `json:"critical_list" yaml:"critical_list"`
}

// defaults：根据等级填充默认强度与开关。
func defaults(c *Config) {
	if c.Obfuscation.Level == "" {
		c.Obfuscation.Level = "medium"
	}
	if c.Passes.EntryExit == false && c.Passes.ConstSplit == false && c.Passes.CFFlatten == false && c.Passes.InstrSubstitute == false && c.Passes.ConstObfuscation == false && c.Passes.Virtualization == false && c.Passes.SecurityHooks == false {
		c.Passes.EntryExit = true
		c.Passes.ConstSplit = true
		c.Passes.CFFlatten = true
		c.Passes.InstrSubstitute = true
		c.Passes.ConstObfuscation = true
		c.Passes.SecurityHooks = true
	}
	switch strings.ToLower(c.Obfuscation.Level) {
	case "low":
		if c.Obfuscation.SubstituteIntensity == 0 {
			c.Obfuscation.SubstituteIntensity = 1
		}
		if c.Obfuscation.FlattenRatio == 0 {
			c.Obfuscation.FlattenRatio = 25
		}
		if c.Obfuscation.VirtualizeRatio == 0 {
			c.Obfuscation.VirtualizeRatio = 5
		}
	case "medium":
		if c.Obfuscation.SubstituteIntensity == 0 {
			c.Obfuscation.SubstituteIntensity = 2
		}
		if c.Obfuscation.FlattenRatio == 0 {
			c.Obfuscation.FlattenRatio = 50
		}
		if c.Obfuscation.VirtualizeRatio == 0 {
			c.Obfuscation.VirtualizeRatio = 10
		}
	case "high":
		if c.Obfuscation.SubstituteIntensity == 0 {
			c.Obfuscation.SubstituteIntensity = 4
		}
		if c.Obfuscation.FlattenRatio == 0 {
			c.Obfuscation.FlattenRatio = 80
		}
		if c.Obfuscation.VirtualizeRatio == 0 {
			c.Obfuscation.VirtualizeRatio = 25
		}
	default:
		c.Obfuscation.Level = "medium"
	}

	// VMP 默认：启用多 VM 支持并提供两套 ISA 标签。
	if len(c.VMP.VMs) == 0 {
		c.VMP.VMs = []VMClass{{Name: "vm_a", ISA: "A"}, {Name: "vm_b", ISA: "B"}}
	}
	if c.VMP.Levels.Normal == "" {
		c.VMP.Levels.Normal = c.VMP.VMs[0].Name
	}
	if c.VMP.Levels.Sensitive == "" {
		c.VMP.Levels.Sensitive = c.VMP.VMs[len(c.VMP.VMs)-1].Name
	}
	if c.VMP.Levels.Critical == "" {
		c.VMP.Levels.Critical = c.VMP.VMs[len(c.VMP.VMs)-1].Name
	}
	if c.VMP.StaticKey == "" {
		c.VMP.StaticKey = "c0ffee42"
	}
	if c.VMP.Levels.FunctionVM == nil {
		c.VMP.Levels.FunctionVM = map[string]string{}
	}
	if c.VMP.Levels.SensitiveList == nil {
		c.VMP.Levels.SensitiveList = []string{}
	}
	if c.VMP.Levels.CriticalList == nil {
		c.VMP.Levels.CriticalList = []string{}
	}
}

// Load 读取 YAML/JSON 配置；空路径则返回默认配置。
func Load(path string) (*Config, error) {
	cfg := &Config{}
	if path == "" {
		defaults(cfg)
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yml", ".yaml":
		err = yaml.Unmarshal(data, cfg)
	default:
		err = json.Unmarshal(data, cfg)
		if err != nil {
			// Attempt YAML as fallback
			err = yaml.Unmarshal(data, cfg)
		}
	}
	if err != nil {
		return nil, err
	}
	defaults(cfg)
	return cfg, nil
}
