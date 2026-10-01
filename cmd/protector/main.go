package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Ero-Cat/Apk-Protector-Tool/internal/app"
	"github.com/Ero-Cat/Apk-Protector-Tool/internal/presets"
	"github.com/Ero-Cat/Apk-Protector-Tool/internal/ui"
)

const deprecationNotice = `warning: flat flag mode is deprecated and will be removed in a future release.
         Prefer subcommands (protector run | scan | sign | ui), or presets (-profile quick|full|sign-only).

`

type sliceFlag []string

func (s *sliceFlag) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}

func (s *sliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// cliOptions 承载全部 CLI 参数；子命令与旧版扁平模式共用同一套注册逻辑。
type cliOptions struct {
	configPath string
	profile    string

	inputAPK    string
	finalOutput string
	workDir     string
	keepWorkDir bool
	reportPath  string
	skipScan    bool

	enableProtection bool
	randomPackage    bool
	multiDexEncrypt  bool
	dexEncrypt       bool
	compressEncrypt  bool
	pseudoEncrypt    bool
	packagePrefix    string
	protectSecret    string
	protectSecretEnv string

	reinforceCmd     string
	reinforceOutput  string
	reinforceTimeout string
	reinforceArgs    sliceFlag
	reinforceEnv     sliceFlag

	zipalignPath  string
	alignment     int
	apksignerPath string

	keystorePath   string
	storePass      string
	storePassEnv   string
	keyPass        string
	keyPassEnv     string
	keyAlias       string
	createKeystore bool
	keytoolPath    string
	signExtraArgs  sliceFlag

	verification bool
}

// registerFlags 在给定 FlagSet 上注册全部参数，并返回解析目标。
func registerFlags(fs *flag.FlagSet) *cliOptions {
	o := &cliOptions{}
	fs.StringVar(&o.configPath, "config", "", "Optional YAML/JSON configuration file")
	fs.StringVar(&o.profile, "profile", "", "Preset baseline applied on top of the config file: quick (scan+align+sign), full (all protections+align+sign+verify) or sign-only (align+sign). Explicit flags still win")
	fs.StringVar(&o.inputAPK, "input", "", "Path to the source APK that needs hardening")
	fs.StringVar(&o.finalOutput, "output", "", "Destination path for the fully processed APK")
	fs.StringVar(&o.workDir, "workdir", "", "Directory used for intermediate artifacts (defaults to the OS temp dir)")
	fs.BoolVar(&o.keepWorkDir, "keep-workdir", false, "Keep the intermediate run directory instead of deleting it")
	fs.StringVar(&o.reportPath, "report", "", "Optional JSON file that captures the execution report")
	fs.BoolVar(&o.skipScan, "skip-scan", false, "Skip built-in APK scanning (加固特征/反环境/SDK/证书等检测)")

	fs.BoolVar(&o.enableProtection, "protect", false, "Enable built-in APK protection steps (package randomization, dex encryption, pseudo encryption). Use -protect=false to force-disable a config-enabled option")
	fs.BoolVar(&o.randomPackage, "protect-random-package", false, "Randomize the manifest package name before reinforcement")
	fs.BoolVar(&o.multiDexEncrypt, "protect-multi-dex", false, "Encrypt all classes*.dex files with AES-GCM")
	fs.BoolVar(&o.dexEncrypt, "protect-dex", false, "Encrypt only the primary classes.dex")
	fs.BoolVar(&o.compressEncrypt, "protect-compress", false, "Compress dex payloads before encryption to shrink APK size")
	fs.BoolVar(&o.pseudoEncrypt, "protect-pseudo", false, "Embed pseudo encrypted artifacts for APK integrity audits")
	fs.StringVar(&o.packagePrefix, "protect-package-prefix", "", "Prefix used when generating random package names")
	fs.StringVar(&o.protectSecret, "protect-secret", "", "Secret used when deriving encryption keys for dex/pseudo encryption")
	fs.StringVar(&o.protectSecretEnv, "protect-secret-env", "", "Name of the environment variable holding the dex encryption secret (keeps secrets out of shell history)")

	fs.StringVar(&o.reinforceCmd, "reinforce-command", "", "Optional command executed before signing, e.g. a third-party hardener CLI")
	fs.StringVar(&o.reinforceOutput, "reinforce-output", "", "Expected APK path produced by the reinforcement command (defaults to an auto-generated file under the workdir)")
	fs.StringVar(&o.reinforceTimeout, "reinforce-timeout", "", "Timeout for the reinforcement command (e.g. 5m)")
	fs.Var(&o.reinforceArgs, "reinforce-arg", "Repeatable flag that passes an argument to the reinforcement command")
	fs.Var(&o.reinforceEnv, "reinforce-env", "Repeatable flag that injects KEY=VALUE env vars into the reinforcement command")

	fs.StringVar(&o.zipalignPath, "zipalign", "", "Path to the zipalign binary; when set the tool performs alignment before signing")
	fs.IntVar(&o.alignment, "align-bytes", 0, "zipalign alignment in bytes (0 = config default, normally 4)")
	fs.StringVar(&o.apksignerPath, "apksigner", "", "Path to the apksigner binary")

	fs.StringVar(&o.keystorePath, "keystore", "", "Java keystore used for signing")
	fs.StringVar(&o.storePass, "store-pass", "", "Password for the keystore (storepass)")
	fs.StringVar(&o.storePassEnv, "store-pass-env", "", "Name of the environment variable holding the keystore password")
	fs.StringVar(&o.keyPass, "key-pass", "", "Password for the private key (keypass)")
	fs.StringVar(&o.keyPassEnv, "key-pass-env", "", "Name of the environment variable holding the key password")
	fs.StringVar(&o.keyAlias, "key-alias", "", "Alias of the signing key within the keystore")
	fs.BoolVar(&o.createKeystore, "create-keystore", false, "Generate the keystore automatically when it is missing (requires keytool)")
	fs.StringVar(&o.keytoolPath, "keytool", "", "Path to the keytool executable (defaults to looking up keytool from PATH)")
	fs.Var(&o.signExtraArgs, "sign-arg", "Repeatable flag that appends a raw argument to apksigner")

	fs.BoolVar(&o.verification, "verify", false, "Run apksigner verify against the final artifact")
	return o
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "ui":
			if err := ui.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "ui:", err)
				os.Exit(1)
			}
			return
		case "run", "scan", "sign":
			os.Exit(runSubcommand(os.Args[1], os.Args[2:]))
		case "config":
			if len(os.Args) > 2 && os.Args[2] == "init" {
				configInit(os.Args[3:])
				return
			}
			fmt.Fprintln(os.Stderr, "usage: protector config init [-o protector.config.yml]")
			os.Exit(2)
		case "help":
			flag.Usage()
			return
		}
	}

	// Legacy flat mode: fully compatible, but deprecated.
	fmt.Fprint(os.Stderr, deprecationNotice)
	os.Exit(runFlat(flag.CommandLine))
}

// runFlat parses legacy flat flags from the global FlagSet.
func runFlat(fs *flag.FlagSet) int {
	fs.Usage = usageFor(fs)
	opts := registerFlags(fs)
	flag.Parse()
	return execute("run", fs, opts)
}

// runSubcommand builds a subcommand FlagSet and executes the pipeline with
// mode-specific forcing.
func runSubcommand(mode string, args []string) int {
	fs := flag.NewFlagSet(mode, flag.ExitOnError)
	fs.Usage = usageFor(fs)
	opts := registerFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return execute(mode, fs, opts)
}

func usageFor(fs *flag.FlagSet) func() {
	return func() {
		out := fs.Output()
		fmt.Fprintf(out, "Usage:\n")
		fmt.Fprintf(out, "  protector run  [options]      full pipeline (config + profile + flags)\n")
		fmt.Fprintf(out, "  protector scan [options]     security scan only\n")
		fmt.Fprintf(out, "  protector sign [options]     align + sign (+ -verify)\n")
		fmt.Fprintf(out, "  protector ui                 interactive wizard\n")
		fmt.Fprintf(out, "  protector config init        write an annotated config template\n")
		fmt.Fprintf(out, "  protector [options]          legacy flat mode (deprecated)\n\n")
		fmt.Fprintln(out, "Paths provided through flags are resolved relative to the current working directory.")
		fmt.Fprintln(out, "Config values support ${VAR} and ${VAR:-default} environment references.")
		fmt.Fprintln(out, "")
		fs.PrintDefaults()
	}
}

// execute loads config, applies profile/flag overrides, enforces the
// subcommand's pipeline shape and runs the tool.
func execute(mode string, fs *flag.FlagSet, opts *cliOptions) int {
	cfg, err := app.LoadConfig(opts.configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if opts.profile != "" {
		profile, err := presets.Parse(opts.profile)
		if err != nil {
			log.Fatalf("profile: %v", err)
		}
		presets.Apply(profile, cfg)
	}

	// Only flags the user actually passed override the config: an explicit
	// -protect=false disables a config-enabled option (P0.5 semantics fix).
	explicit := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

	if err := applyOverrides(cfg, opts, explicit); err != nil {
		log.Fatalf("apply flags: %v", err)
	}

	// Subcommand intent is strongest: it runs after every override layer.
	switch mode {
	case "scan":
		cfg.Scanning.Enabled = true
		cfg.Protections = app.ProtectionConfig{Enabled: false}
		cfg.Reinforce.Enabled = false
		cfg.Zipalign.Enabled = false
		cfg.Signing.Enabled = false
		cfg.Verification.Enabled = false
	case "sign":
		cfg.Scanning.Enabled = false
		cfg.Protections = app.ProtectionConfig{Enabled: false}
		cfg.Reinforce.Enabled = false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	tool := app.NewTool(cfg)
	report, err := tool.Run(ctx)
	if err != nil {
		log.Fatalf("execution failed: %v", err)
	}

	if cfg.Reporting.JSON != "" {
		if err := app.WriteReport(cfg.Reporting.JSON, report); err != nil {
			log.Fatalf("write report: %v", err)
		}
	}

	fmt.Printf("Successfully processed APK. Final artifact: %s\\n", report.FinalAPK)
	if report.Hashes["sha256"] != "" {
		fmt.Printf("SHA256: %s\\n", report.Hashes["sha256"])
	}
	return 0
}

// lookupEnvValue resolves an env-name flag; an unset variable is a hard error
// naming both the variable and what it was needed for.
func lookupEnvValue(name, purpose string) (string, error) {
	value, ok := os.LookupEnv(name)
	if !ok || value == "" {
		return "", fmt.Errorf("environment variable %s (%s) is not set — export it before running protector", name, purpose)
	}
	return value, nil
}

// applyOverrides layers CLI flags on top of the config. Bool flags apply only
// when explicitly passed (either polarity); string flags apply when non-empty.
func applyOverrides(cfg *app.Config, o *cliOptions, explicit map[string]bool) error {
	// Env-name flags resolve at startup so secrets never appear on the
	// command line or in shell history.
	if o.protectSecretEnv != "" {
		value, err := lookupEnvValue(o.protectSecretEnv, "dex encryption secret")
		if err != nil {
			return err
		}
		cfg.Protections.EncryptionSecret = value
	}
	if o.storePassEnv != "" {
		value, err := lookupEnvValue(o.storePassEnv, "keystore password")
		if err != nil {
			return err
		}
		cfg.Signing.StorePass = value
	}
	if o.keyPassEnv != "" {
		value, err := lookupEnvValue(o.keyPassEnv, "key password")
		if err != nil {
			return err
		}
		cfg.Signing.KeyPass = value
	}

	setBool := func(name string, target *bool, value bool) {
		if explicit[name] {
			*target = value
		}
	}

	if o.inputAPK != "" {
		cfg.InputAPK = o.inputAPK
	}
	if o.finalOutput != "" {
		cfg.FinalOutput = o.finalOutput
	}
	if o.workDir != "" {
		cfg.WorkDir = o.workDir
	}
	setBool("keep-workdir", &cfg.KeepWorkDir, o.keepWorkDir)
	if o.skipScan {
		cfg.Scanning.Enabled = false
	}

	// 显式 -protect=false 是强意图：清掉未显式指定的子开关，否则 finalize
	// 会因配置里的子开关隐式重新启用保护；显式传入的子开关由下方 setBool
	// 按用户取值重新应用。
	if explicit["protect"] && !o.enableProtection {
		cfg.Protections.Enabled = false
		if !explicit["protect-random-package"] {
			cfg.Protections.RandomPackage = false
		}
		if !explicit["protect-multi-dex"] {
			cfg.Protections.MultiDexEncrypt = false
		}
		if !explicit["protect-dex"] {
			cfg.Protections.DexEncrypt = false
		}
		if !explicit["protect-pseudo"] {
			cfg.Protections.PseudoEncrypt = false
		}
		if !explicit["protect-compress"] {
			cfg.Protections.CompressBeforeEncrypt = false
		}
	} else {
		setBool("protect", &cfg.Protections.Enabled, o.enableProtection)
	}
	setBool("protect-random-package", &cfg.Protections.RandomPackage, o.randomPackage)
	setBool("protect-multi-dex", &cfg.Protections.MultiDexEncrypt, o.multiDexEncrypt)
	setBool("protect-dex", &cfg.Protections.DexEncrypt, o.dexEncrypt)
	setBool("protect-compress", &cfg.Protections.CompressBeforeEncrypt, o.compressEncrypt)
	setBool("protect-pseudo", &cfg.Protections.PseudoEncrypt, o.pseudoEncrypt)
	if o.packagePrefix != "" {
		cfg.Protections.PackagePrefix = o.packagePrefix
	}
	if o.protectSecret != "" {
		cfg.Protections.EncryptionSecret = o.protectSecret
	}

	if o.reinforceCmd != "" {
		cfg.Reinforce.Enabled = true
		cfg.Reinforce.Command = o.reinforceCmd
	}
	if o.reinforceOutput != "" {
		cfg.Reinforce.OutputAPK = o.reinforceOutput
	}
	if o.reinforceTimeout != "" {
		if _, err := time.ParseDuration(o.reinforceTimeout); err != nil {
			return fmt.Errorf("-reinforce-timeout: %v", err)
		}
		cfg.Reinforce.Timeout = o.reinforceTimeout
	}
	if len(o.reinforceArgs) > 0 {
		cfg.Reinforce.Args = append([]string{}, o.reinforceArgs...)
	}
	if len(o.reinforceEnv) > 0 {
		if cfg.Reinforce.Env == nil {
			cfg.Reinforce.Env = map[string]string{}
		}
		for _, kv := range o.reinforceEnv {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) != 2 {
				continue
			}
			cfg.Reinforce.Env[parts[0]] = parts[1]
		}
	}

	if o.zipalignPath != "" {
		cfg.Zipalign.Enabled = true
		cfg.Zipalign.Path = o.zipalignPath
	}
	if o.alignment > 0 {
		cfg.Zipalign.Alignment = o.alignment
	}
	if o.apksignerPath != "" {
		cfg.Signing.ApksignerPath = o.apksignerPath
	}

	if o.keystorePath != "" {
		cfg.Signing.Keystore = o.keystorePath
	}
	if o.storePass != "" {
		cfg.Signing.StorePass = o.storePass
	}
	if o.keyPass != "" {
		cfg.Signing.KeyPass = o.keyPass
	}
	if o.keyAlias != "" {
		cfg.Signing.KeyAlias = o.keyAlias
	}
	setBool("create-keystore", &cfg.Signing.CreateKeystore, o.createKeystore)
	if o.keytoolPath != "" {
		cfg.Signing.KeytoolPath = o.keytoolPath
	}
	if len(o.signExtraArgs) > 0 {
		cfg.Signing.AdditionalArgs = append([]string{}, o.signExtraArgs...)
	}

	if o.reportPath != "" {
		cfg.Reporting.JSON = o.reportPath
	}
	setBool("verify", &cfg.Verification.Enabled, o.verification)
	return nil
}
