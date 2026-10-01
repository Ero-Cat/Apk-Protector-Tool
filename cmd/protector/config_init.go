package main

import (
	"flag"
	"fmt"
	"os"
)

// configTemplate 是 `protector config init` 写出的带注释配置模板。
// 密钥一律使用 ${VAR} 环境变量引用，避免明文落盘。
const configTemplate = `# protector 配置模板（由 protector config init 生成）
# 用法: protector run -config protector.config.yml -profile full
# 字符串值支持 ${VAR} 与 ${VAR:-default} 环境变量引用；未设置且无默认值会在加载时报错。

# 输入 APK（必填）
input_apk: app-release.apk
# 最终产物路径（缺省 dist/<名称>-protected.apk）
final_output: dist/app-protected.apk
# 中间产物根目录（缺省系统临时目录）；keep_work_dir 便于排查
work_dir: ""
keep_work_dir: false

protections:
  enabled: true
  # 等长随机化清单包名
  random_package: true
  package_prefix: com.protector
  # 加密全部 classes*.dex（protections 主开关 + 本项即可）
  multi_dex_encrypt: true
  # 仅加密主 dex（与 multi_dex_encrypt 二选一）
  dex_encrypt: false
  # 伪加固标记：内嵌模拟主流加固器的特征产物
  pseudo_encrypt: true
  # 加密前 Deflate 压缩，产物更小
  compress_before_encrypt: true
  # 加密密钥：强烈建议走环境变量引用，不要明文
  encryption_secret: "${APK_PROTECT_SECRET}"
  # legacy 逃生开关（不推荐）：把密钥内嵌进 APK 内 metadata.json。
  # 默认 false——密钥写 <final_output>.key（0600），经发布渠道注入加载器，
  # 见 docs/design/adr-0001-dex-key-delivery.md
  embed_key: false

scanning:
  enabled: true
  # apksigner 路径（缺省取 signing.apksigner_path 或 PATH）
  apksigner_path: ""
  # 反环境关键词（缺省 frida/xposed/magisk/root 等内置表）
  keywords: []
  key_leak_hints: []
  max_scan_size_mb: 4

# 第三方加固器（可选）：签名前执行的外部 CLI
reinforce:
  enabled: false
  command: ""
  args: []
  env: {}
  # 例 "5m"
  timeout: ""
  output_apk: ""
  skip_if_output_exists: false

zipalign:
  enabled: true
  # 路径可以引用环境变量：${ANDROID_HOME}/build-tools/36.1.0/zipalign
  path: "zipalign"
  alignment: 4
  output_apk: ""

signing:
  enabled: true
  apksigner_path: "apksigner"
  keystore: "sign/release.keystore"
  key_alias: "release"
  # 密码一律走环境变量引用
  store_pass: "${APK_STORE_PASS}"
  key_pass: ""   # 缺省回落 store_pass
  output_apk: ""
  # keystore 缺失时经 keytool 自动生成
  create_keystore: false
  keytool_path: "keytool"
  key_algorithm: "RSA"
  key_size: 2048
  validity_days: 3650
  distinguished_name: "CN=Android Hardening,O=Automation,OU=Security,L=Unknown,ST=Unknown,C=CN"
  additional_args: []

verification:
  enabled: true

reporting:
  json: "dist/report.json"
`

// configInit 实现 `protector config init [-o path] [-f]`。
func configInit(args []string) {
	fs := flag.NewFlagSet("config init", flag.ExitOnError)
	output := fs.String("o", "protector.config.yml", "Path of the config template to write")
	force := fs.Bool("f", false, "Overwrite the target file if it exists")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	if !*force {
		if _, err := os.Stat(*output); err == nil {
			fmt.Fprintf(os.Stderr, "refusing to overwrite existing %s — pass -f to force or -o <path> to choose another file\n", *output)
			os.Exit(1)
		}
	}
	if err := os.WriteFile(*output, []byte(configTemplate), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", *output, err)
		os.Exit(1)
	}
	fmt.Printf("Configuration template written to %s\n", *output)
	fmt.Println("Next steps:")
	fmt.Printf("  1) export APK_STORE_PASS=... APK_PROTECT_SECRET=...\n")
	fmt.Printf("  2) edit %s (input_apk, keystore, paths)\n", *output)
	fmt.Printf("  3) protector run -config %s\n", *output)
}
