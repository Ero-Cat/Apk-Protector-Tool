package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"protector-tool/internal/app"
)

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

func main() {
	var (
		configPath       = flag.String("config", "", "Optional YAML/JSON configuration file")
		inputAPK         = flag.String("input", "", "Path to the source APK that needs hardening")
		finalOutput      = flag.String("output", "", "Destination path for the fully processed APK")
		workDir          = flag.String("workdir", "", "Directory used for intermediate artifacts (defaults to the OS temp dir)")
		enableProtection = flag.Bool("protect", false, "Enable built-in APK protection steps (package randomization, dex encryption, pseudo encryption)")
		randomPackage    = flag.Bool("protect-random-package", false, "Randomize the manifest package name before reinforcement")
		multiDexEncrypt  = flag.Bool("protect-multi-dex", false, "Encrypt all classes*.dex files with AES-GCM")
		dexEncrypt       = flag.Bool("protect-dex", false, "Encrypt only the primary classes.dex")
		compressEncrypt  = flag.Bool("protect-compress", false, "Compress dex payloads before encryption to shrink APK size")
		pseudoEncrypt    = flag.Bool("protect-pseudo", false, "Embed pseudo encrypted artifacts for APK integrity audits")
		packagePrefix    = flag.String("protect-package-prefix", "", "Prefix used when generating random package names")
		protectSecret    = flag.String("protect-secret", "", "Secret used when deriving encryption keys for dex/pseudo encryption")
		reinforceCmd     = flag.String("reinforce-command", "", "Optional command executed before signing, e.g. a third-party hardener CLI")
		reinforceOutput  = flag.String("reinforce-output", "", "Expected APK path produced by the reinforcement command (defaults to an auto-generated file under the workdir)")
		zipalignPath     = flag.String("zipalign", "", "Path to the zipalign binary; when set the tool performs alignment before signing")
		apksignerPath    = flag.String("apksigner", "", "Path to the apksigner binary")
		keystorePath     = flag.String("keystore", "", "Java keystore used for signing")
		storePass        = flag.String("store-pass", "", "Password for the keystore (storepass)")
		keyPass          = flag.String("key-pass", "", "Password for the private key (keypass)")
		keyAlias         = flag.String("key-alias", "", "Alias of the signing key within the keystore")
		createKeystore   = flag.Bool("create-keystore", false, "Generate the keystore automatically when it is missing (requires keytool)")
		keytoolPath      = flag.String("keytool", "", "Path to the keytool executable (defaults to looking up keytool from PATH)")
		skipScan         = flag.Bool("skip-scan", false, "Skip built-in APK scanning (加固特征/反环境/SDK/证书等检测)")
		reportPath       = flag.String("report", "", "Optional JSON file that captures the execution report")
		verificationStep = flag.Bool("verify", false, "Run apksigner verify against the final artifact")
	)

	var reinforceArgs sliceFlag
	flag.Var(&reinforceArgs, "reinforce-arg", "Repeatable flag that passes an argument to the reinforcement command")
	var reinforceEnv sliceFlag
	flag.Var(&reinforceEnv, "reinforce-env", "Repeatable flag that injects KEY=VALUE env vars into the reinforcement command")
	var signingExtraArgs sliceFlag
	flag.Var(&signingExtraArgs, "sign-arg", "Repeatable flag that appends a raw argument to apksigner")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options]\\n\\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "The tool automates APK reinforcement (via an optional command) and V1+V2 signing.")
		fmt.Fprintln(flag.CommandLine.Output(), "Paths provided through flags are resolved relative to the current working directory.")
		fmt.Fprintln(flag.CommandLine.Output(), "")
		flag.PrintDefaults()
	}

	flag.Parse()

	cfg, err := app.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	applyOverrides(cfg, applyArgs{
		inputAPK:         *inputAPK,
		finalOutput:      *finalOutput,
		workDir:          *workDir,
		enableProtection: *enableProtection,
		randomPackage:    *randomPackage,
		multiDexEncrypt:  *multiDexEncrypt,
		dexEncrypt:       *dexEncrypt,
		compressEncrypt:  *compressEncrypt,
		pseudoEncrypt:    *pseudoEncrypt,
		packagePrefix:    *packagePrefix,
		protectSecret:    *protectSecret,
		reinforceCmd:     *reinforceCmd,
		reinforceArgs:    reinforceArgs,
		reinforceEnv:     reinforceEnv,
		reinforceOutput:  *reinforceOutput,
		zipalignPath:     *zipalignPath,
		apksignerPath:    *apksignerPath,
		keystorePath:     *keystorePath,
		storePass:        *storePass,
		keyPass:          *keyPass,
		keyAlias:         *keyAlias,
		createKeystore:   *createKeystore,
		keytoolPath:      *keytoolPath,
		skipScan:         *skipScan,
		reportPath:       *reportPath,
		signingExtraArgs: signingExtraArgs,
		verification:     *verificationStep,
	})

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
}

type applyArgs struct {
	inputAPK         string
	finalOutput      string
	workDir          string
	skipScan         bool
	enableProtection bool
	randomPackage    bool
	multiDexEncrypt  bool
	dexEncrypt       bool
	compressEncrypt  bool
	pseudoEncrypt    bool
	packagePrefix    string
	protectSecret    string
	reinforceCmd     string
	reinforceArgs    []string
	reinforceEnv     []string
	reinforceOutput  string
	zipalignPath     string
	apksignerPath    string
	keystorePath     string
	storePass        string
	keyPass          string
	keyAlias         string
	createKeystore   bool
	keytoolPath      string
	reportPath       string
	signingExtraArgs []string
	verification     bool
}

func applyOverrides(cfg *app.Config, args applyArgs) {
	if args.inputAPK != "" {
		cfg.InputAPK = args.inputAPK
	}
	if args.finalOutput != "" {
		cfg.FinalOutput = args.finalOutput
	}
	if args.workDir != "" {
		cfg.WorkDir = args.workDir
	}
	if args.enableProtection {
		cfg.Protections.Enabled = true
	}
	if args.skipScan {
		cfg.Scanning.Enabled = false
	}
	if args.randomPackage {
		cfg.Protections.RandomPackage = true
	}
	if args.multiDexEncrypt {
		cfg.Protections.MultiDexEncrypt = true
	}
	if args.dexEncrypt {
		cfg.Protections.DexEncrypt = true
	}
	if args.compressEncrypt {
		cfg.Protections.CompressBeforeEncrypt = true
	}
	if args.pseudoEncrypt {
		cfg.Protections.PseudoEncrypt = true
	}
	if args.packagePrefix != "" {
		cfg.Protections.PackagePrefix = args.packagePrefix
	}
	if args.protectSecret != "" {
		cfg.Protections.EncryptionSecret = args.protectSecret
	}
	if args.reinforceCmd != "" {
		cfg.Reinforce.Enabled = true
		cfg.Reinforce.Command = args.reinforceCmd
	}
	if len(args.reinforceArgs) > 0 {
		cfg.Reinforce.Args = append([]string{}, args.reinforceArgs...)
	}
	if len(args.reinforceEnv) > 0 {
		if cfg.Reinforce.Env == nil {
			cfg.Reinforce.Env = map[string]string{}
		}
		for _, kv := range args.reinforceEnv {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) != 2 {
				continue
			}
			cfg.Reinforce.Env[parts[0]] = parts[1]
		}
	}
	if args.reinforceOutput != "" {
		cfg.Reinforce.OutputAPK = args.reinforceOutput
	}
	if args.zipalignPath != "" {
		cfg.Zipalign.Enabled = true
		cfg.Zipalign.Path = args.zipalignPath
	}
	if args.apksignerPath != "" {
		cfg.Signing.ApksignerPath = args.apksignerPath
	}
	if args.keystorePath != "" {
		cfg.Signing.Keystore = args.keystorePath
	}
	if args.storePass != "" {
		cfg.Signing.StorePass = args.storePass
	}
	if args.keyPass != "" {
		cfg.Signing.KeyPass = args.keyPass
	}
	if args.keyAlias != "" {
		cfg.Signing.KeyAlias = args.keyAlias
	}
	if args.createKeystore {
		cfg.Signing.CreateKeystore = true
	}
	if args.keytoolPath != "" {
		cfg.Signing.KeytoolPath = args.keytoolPath
	}
	if args.reportPath != "" {
		cfg.Reporting.JSON = args.reportPath
	}
	if len(args.signingExtraArgs) > 0 {
		cfg.Signing.AdditionalArgs = append([]string{}, args.signingExtraArgs...)
	}
	if args.verification {
		cfg.Verification.Enabled = true
	}
}
