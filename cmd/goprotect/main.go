// goprotect CLI：读取配置，按序执行 IR Pass，并输出变换后的 bitcode 与报告。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"protector-tool/config"
	"protector-tool/llvmwrap"
	"protector-tool/passes"
	"protector-tool/report"
)

func main() {
	cfgPath := flag.String("config", "", "Path to YAML/JSON config")
	input := flag.String("input", "", "Input LLVM bitcode/IR (.bc/.ll)")
	output := flag.String("o", "", "Output LLVM bitcode/IR")
	level := flag.String("level", "", "Override obfuscation level (low|medium|high)")
	dumpCFG := flag.Bool("dump-cfg", false, "Dump DOT graphs before/after passes")
	flag.Parse()

	if *input == "" {
		log.Fatalf("input is required (use --input)")
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if *level != "" {
		cfg.Obfuscation.Level = *level
	}
	if *output != "" {
		cfg.Output = *output
	}
	if *dumpCFG {
		cfg.Debug.DumpCFG = true
	}
	// Always trust CLI input path.
	cfg.Input = *input

	if !llvmwrap.HasNative() {
		log.Fatalf("goprotect was built without LLVM support. Rebuild with `-tags llvm` and ensure llvm-config is available.")
	}

	mod, err := llvmwrap.ParseBitcode(cfg.Input)
	if err != nil {
		log.Fatalf("parse bitcode: %v", err)
	}
	defer mod.Dispose()

	rpt := report.New()
	pipeline := passes.BuildPipeline(cfg, rpt)
	if err := pipeline.Run(mod); err != nil {
		log.Fatalf("pipeline: %v", err)
	}

	if cfg.Output == "" {
		base := filepath.Base(cfg.Input)
		cfg.Output = filepath.Join(filepath.Dir(cfg.Input), "obf-"+base)
	}
	if err := mod.WriteBitcode(cfg.Output); err != nil {
		log.Fatalf("write output: %v", err)
	}

	if cfg.Report.Path != "" {
		if err := rpt.Write(cfg.Report.Path); err != nil {
			log.Printf("warn: write report: %v", err)
		}
	}

	// Print a short summary to stdout so Android build logs capture it.
	summary, _ := json.MarshalIndent(rpt, "", "  ")
	fmt.Printf("[goprotect] wrote %s\n", cfg.Output)
	fmt.Printf("[goprotect] summary:\n%s\n", string(summary))
}
