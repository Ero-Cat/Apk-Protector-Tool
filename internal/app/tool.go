package app

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Tool orchestrates the APK reinforcement and signing workflow.
type Tool struct {
	cfg *Config

	// Progress is invoked at the start of each pipeline stage (scan, protect,
	// reinforce, align, sign, verify, finalize). It is optional and mainly
	// consumed by the interactive UI.
	Progress func(stage string)

	// Stdout and Stderr receive output produced by external commands such as
	// zipalign and apksigner. They default to the process streams when nil;
	// the interactive UI injects buffers so that child output does not tear
	// the rendered screen apart.
	Stdout io.Writer
	Stderr io.Writer
}

// NewTool creates a Tool instance.
func NewTool(cfg *Config) *Tool {
	return &Tool{cfg: cfg}
}

func (t *Tool) progress(stage string) {
	if t.Progress != nil {
		t.Progress(stage)
	}
}

func (t *Tool) stdout() io.Writer {
	if t.Stdout != nil {
		return t.Stdout
	}
	return os.Stdout
}

func (t *Tool) stderr() io.Writer {
	if t.Stderr != nil {
		return t.Stderr
	}
	return os.Stderr
}

// ReportStep captures the status of each pipeline stage.
type ReportStep struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Artifact string `json:"artifact,omitempty"`
	Details  string `json:"details,omitempty"`
}

// Report represents the final execution summary.
type Report struct {
	InputAPK   string            `json:"input_apk"`
	FinalAPK   string            `json:"final_apk"`
	WorkDir    string            `json:"work_dir"`
	StartedAt  time.Time         `json:"started_at"`
	FinishedAt time.Time         `json:"finished_at"`
	Duration   string            `json:"duration"`
	Steps      []ReportStep      `json:"steps"`
	Artifacts  map[string]string `json:"artifacts"`
	Hashes     map[string]string `json:"hashes"`
	Scan       *ScanResults      `json:"scan,omitempty"`
}

// WriteReport saves the report as JSON.
func WriteReport(path string, report *Report) error {
	if report == nil {
		return fmt.Errorf("report is nil")
	}
	if path == "" {
		return fmt.Errorf("report path is empty")
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create report dir: %w", err)
		}
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// Run executes the pipeline.
func (t *Tool) Run(ctx context.Context) (*Report, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}
	if err := t.cfg.finalize(cwd); err != nil {
		return nil, err
	}
	if _, err := os.Stat(t.cfg.InputAPK); err != nil {
		return nil, fmt.Errorf("input apk: %w", err)
	}

	if err := t.preflight(t.cfg.InputAPK); err != nil {
		return nil, err
	}

	baseName := strings.TrimSuffix(filepath.Base(t.cfg.InputAPK), filepath.Ext(t.cfg.InputAPK))

	workRoot := t.cfg.WorkDir
	if workRoot != "" {
		if err := os.MkdirAll(workRoot, 0o755); err != nil {
			return nil, fmt.Errorf("create workdir: %w", err)
		}
	}
	runDir, err := os.MkdirTemp(workRoot, "protector-")
	if err != nil {
		return nil, fmt.Errorf("create temp workdir: %w", err)
	}
	if !t.cfg.KeepWorkDir {
		defer os.RemoveAll(runDir)
	}

	report := &Report{
		InputAPK:  t.cfg.InputAPK,
		WorkDir:   runDir,
		StartedAt: time.Now(),
		Steps:     []ReportStep{},
		Artifacts: map[string]string{},
		Hashes:    map[string]string{},
	}
	defer func() {
		report.FinishedAt = time.Now()
		report.Duration = time.Since(report.StartedAt).String()
	}()

	currentApk := filepath.Join(runDir, filepath.Base(t.cfg.InputAPK))
	if err := copyFile(t.cfg.InputAPK, currentApk); err != nil {
		return report, fmt.Errorf("copy input apk: %w", err)
	}
	report.Artifacts["copied_apk"] = currentApk

	if err := t.runScanning(currentApk, report); err != nil {
		return report, err
	}

	globalPlaceholders := map[string]string{
		"work_dir": runDir,
		"run_dir":  runDir,
		"ts":       report.StartedAt.Format("20060102T150405"),
	}

	t.progress("protect")
	protectedApk, err := t.runProtections(currentApk, baseName, runDir, report)
	if err != nil {
		return report, err
	}
	if protectedApk != "" {
		currentApk = protectedApk
	}

	reinforced, err := t.runReinforce(ctx, currentApk, baseName, runDir, globalPlaceholders, report)
	if err != nil {
		return report, err
	}
	if reinforced != "" {
		currentApk = reinforced
	}

	aligned, err := t.runZipalign(ctx, currentApk, baseName, runDir, globalPlaceholders, report)
	if err != nil {
		return report, err
	}
	if aligned != "" {
		currentApk = aligned
	}

	signed, err := t.runSigning(ctx, currentApk, baseName, runDir, report)
	if err != nil {
		return report, err
	}
	if signed != "" {
		currentApk = signed
	}

	if t.cfg.Verification.Enabled {
		if err := t.runVerification(ctx, currentApk, report); err != nil {
			return report, err
		}
	}

	t.progress("finalize")
	finalOutput := t.cfg.FinalOutput
	if finalOutput == "" {
		finalOutput = filepath.Join(cwd, "dist", baseName+"-protected.apk")
	}
	if err := os.MkdirAll(filepath.Dir(finalOutput), 0o755); err != nil {
		return report, fmt.Errorf("create output dir: %w", err)
	}
	if err := copyFile(currentApk, finalOutput); err != nil {
		return report, fmt.Errorf("copy final apk: %w", err)
	}
	report.FinalAPK = finalOutput
	t.cfg.FinalOutput = finalOutput
	report.Artifacts["final_apk"] = finalOutput

	if hash, err := computeSHA256(finalOutput); err == nil {
		report.Hashes["sha256"] = hash
	}

	return report, nil
}

// transformsOutput reports whether any stage will rewrite the APK. Scan-only
// runs copy the input unchanged, so implicit zipalign does not apply.
func (t *Tool) transformsOutput() bool {
	return t.cfg.Protections.Enabled || t.cfg.Reinforce.Enabled || t.cfg.Signing.Enabled
}

// preflight verifies that every external binary the run will need actually
// exists before any work starts, so failures surface as actionable errors
// instead of a late crash in the middle of the pipeline. It also covers the
// implicit zipalign requirement for APKs that ship native libs.
func (t *Tool) preflight(inputAPK string) error {
	if t.cfg.Signing.Enabled {
		if err := checkBinary(t.cfg.Signing.ApksignerPath); err != nil {
			return fmt.Errorf("signing: %w", err)
		}
		if !fileExists(t.cfg.Signing.Keystore) && t.cfg.Signing.CreateKeystore {
			if err := checkBinary(t.cfg.Signing.KeytoolPath); err != nil {
				return fmt.Errorf("keystore generation: %w", err)
			}
		}
	}
	needAlign := t.cfg.Zipalign.Enabled
	if !needAlign && t.transformsOutput() {
		hasNativeLibs, err := apkContainsNativeLibs(inputAPK)
		if err != nil {
			return fmt.Errorf("inspect apk for zipalign: %w", err)
		}
		needAlign = hasNativeLibs
	}
	if needAlign {
		if err := checkBinary(t.cfg.Zipalign.Path); err != nil {
			return fmt.Errorf("alignment (required for APKs with native libs): %w", err)
		}
	}
	return nil
}

// checkBinary validates an external tool path: explicit paths must exist,
// bare names are looked up on PATH.
func checkBinary(name string) error {
	if name == "" {
		return fmt.Errorf("tool path is empty")
	}
	if strings.ContainsRune(name, os.PathSeparator) || strings.ContainsRune(name, '/') {
		if _, err := os.Stat(name); err != nil {
			return fmt.Errorf("%s not found — install Android build-tools or set an explicit path", name)
		}
		return nil
	}
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%q not found in PATH — install Android build-tools or set an explicit path via config/flags", name)
	}
	return nil
}

func (t *Tool) runScanning(apkPath string, report *Report) error {
	if !t.cfg.Scanning.Enabled {
		return nil
	}
	t.progress("scan")
	step := ReportStep{Name: "scan"}
	defer func() {
		report.Steps = append(report.Steps, step)
	}()
	results, err := scanAPK(apkPath, t.cfg.Scanning)
	if err != nil {
		step.Status = "failed"
		step.Details = err.Error()
		return fmt.Errorf("scan apk: %w", err)
	}
	report.Scan = results
	step.Status = "completed"
	return nil
}

func (t *Tool) runReinforce(ctx context.Context, currentApk, baseName, runDir string, baseVars map[string]string, report *Report) (string, error) {
	cfg := t.cfg.Reinforce
	if !cfg.Enabled || cfg.Command == "" {
		return "", nil
	}
	step := ReportStep{Name: "reinforce"}
	defer func() {
		report.Steps = append(report.Steps, step)
	}()

	if !cfg.Enabled || cfg.Command == "" {
		return "", nil
	}
	t.progress("reinforce")

	output := cfg.OutputAPK
	if output == "" {
		output = filepath.Join(runDir, baseName+"-reinforced.apk")
	}

	if cfg.SkipIfOutputExists && fileExists(output) {
		step.Status = "skipped"
		step.Artifact = output
		return output, nil
	}

	vars := cloneMap(baseVars)
	vars["input_apk"] = currentApk
	vars["output_apk"] = output

	args := applyTemplates(cfg.Args, vars)
	command := applyTemplate(cfg.Command, vars)
	env := expandEnv(cfg.Env, vars)

	timeout, err := parseTimeout(cfg.Timeout)
	if err != nil {
		return "", err
	}
	if err := t.runCommand(ctx, command, args, env, timeout); err != nil {
		step.Status = "failed"
		step.Details = err.Error()
		return "", fmt.Errorf("reinforce command: %w", err)
	}
	if !fileExists(output) {
		step.Status = "failed"
		step.Details = "expected output not found"
		return "", fmt.Errorf("reinforce output %s missing", output)
	}

	step.Status = "completed"
	step.Artifact = output
	report.Artifacts["reinforced_apk"] = output
	return output, nil
}

func (t *Tool) runZipalign(ctx context.Context, currentApk, baseName, runDir string, baseVars map[string]string, report *Report) (string, error) {
	cfg := t.cfg.Zipalign
	autoEnabled := false
	if !cfg.Enabled {
		if !t.transformsOutput() {
			// Scan-only 等纯复制运行：产物不变，无需对齐。
			return "", nil
		}
		hasNativeLibs, err := apkContainsNativeLibs(currentApk)
		if err != nil {
			return "", fmt.Errorf("inspect apk for zipalign: %w", err)
		}
		if !hasNativeLibs {
			return "", nil
		}
		autoEnabled = true
	}
	t.progress("align")
	step := ReportStep{Name: "zipalign"}
	if autoEnabled {
		step.Name = "zipalign_auto"
	}
	defer func() {
		report.Steps = append(report.Steps, step)
	}()

	output := cfg.OutputAPK
	if output == "" {
		output = filepath.Join(runDir, baseName+"-aligned.apk")
	}

	path := cfg.Path
	if path == "" {
		path = "zipalign"
	}
	alignment := cfg.Alignment
	if alignment <= 0 {
		alignment = 4
	}
	args := []string{"-p", "-f", strconv.Itoa(alignment), currentApk, output}
	if err := t.runCommand(ctx, path, args, nil, 0); err != nil {
		step.Status = "failed"
		step.Details = err.Error()
		if autoEnabled {
			return "", fmt.Errorf("zipalign is required for APKs with native libs (lib/*.so): %w", err)
		}
		return "", fmt.Errorf("zipalign: %w", err)
	}
	if !fileExists(output) {
		step.Status = "failed"
		step.Details = "zipalign output missing"
		return "", fmt.Errorf("zipalign output %s missing", output)
	}

	step.Status = "completed"
	step.Artifact = output
	report.Artifacts["zipaligned_apk"] = output
	return output, nil
}

func apkContainsNativeLibs(apkPath string) (bool, error) {
	reader, err := zip.OpenReader(apkPath)
	if err != nil {
		return false, err
	}
	defer reader.Close()
	for _, file := range reader.File {
		if strings.HasPrefix(file.Name, "lib/") && strings.HasSuffix(file.Name, ".so") {
			return true, nil
		}
	}
	return false, nil
}

func (t *Tool) runSigning(ctx context.Context, currentApk, baseName, runDir string, report *Report) (string, error) {
	cfg := t.cfg.Signing
	if !cfg.Enabled {
		return "", nil
	}
	t.progress("sign")
	step := ReportStep{Name: "sign"}
	defer func() {
		report.Steps = append(report.Steps, step)
	}()

	output := cfg.OutputAPK
	if output == "" {
		output = filepath.Join(runDir, baseName+"-signed.apk")
	}

	if err := t.ensureKeystore(ctx, cfg); err != nil {
		step.Status = "failed"
		step.Details = err.Error()
		return "", err
	}

	args := []string{
		"sign",
		"--ks", cfg.Keystore,
		"--ks-key-alias", cfg.KeyAlias,
		"--ks-pass", "pass:" + cfg.StorePass,
		"--key-pass", "pass:" + cfg.KeyPass,
		"--v1-signing-enabled", "true",
		"--v2-signing-enabled", "true",
		"--out", output,
	}
	args = append(args, cfg.AdditionalArgs...)
	args = append(args, currentApk)

	if err := t.runCommand(ctx, cfg.ApksignerPath, args, nil, 0); err != nil {
		step.Status = "failed"
		step.Details = err.Error()
		return "", fmt.Errorf("apksigner: %w", err)
	}
	if !fileExists(output) {
		step.Status = "failed"
		step.Details = "apksigner output missing"
		return "", fmt.Errorf("signed output %s missing", output)
	}

	step.Status = "completed"
	step.Artifact = output
	report.Artifacts["signed_apk"] = output
	return output, nil
}

func (t *Tool) runVerification(ctx context.Context, apk string, report *Report) error {
	t.progress("verify")
	step := ReportStep{Name: "verify"}
	defer func() {
		report.Steps = append(report.Steps, step)
	}()

	args := []string{
		"verify",
		"--print-certs",
		apk,
	}
	if err := t.runCommand(ctx, t.cfg.Signing.ApksignerPath, args, nil, 0); err != nil {
		step.Status = "failed"
		step.Details = err.Error()
		return fmt.Errorf("apksigner verify: %w", err)
	}
	step.Status = "completed"
	step.Artifact = apk
	return nil
}

func (t *Tool) ensureKeystore(ctx context.Context, cfg SigningConfig) error {
	if fileExists(cfg.Keystore) {
		return nil
	}
	if !cfg.CreateKeystore {
		return fmt.Errorf("keystore %s does not exist and auto creation is disabled", cfg.Keystore)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Keystore), 0o755); err != nil {
		return fmt.Errorf("create keystore dir: %w", err)
	}

	args := []string{
		"-genkeypair",
		"-v",
		"-keystore", cfg.Keystore,
		"-storepass", cfg.StorePass,
		"-keypass", cfg.KeyPass,
		"-alias", cfg.KeyAlias,
		"-keyalg", cfg.KeyAlgorithm,
		"-keysize", strconv.Itoa(cfg.KeySize),
		"-validity", strconv.Itoa(cfg.ValidityDays),
		"-dname", cfg.DistinguishedName,
	}
	return t.runCommand(ctx, cfg.KeytoolPath, args, nil, 0)
}

func parseTimeout(value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout %q: %w", value, err)
	}
	return d, nil
}

func (t *Tool) runCommand(ctx context.Context, bin string, args []string, env map[string]string, timeout time.Duration) error {
	if bin == "" {
		return fmt.Errorf("command path is empty")
	}
	cctx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		cctx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		cctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	cmd := exec.CommandContext(cctx, bin, args...)
	cmd.Stdout = t.stdout()
	cmd.Stderr = t.stderr()
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), formatEnv(env)...)
	}
	return cmd.Run()
}

func formatEnv(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	return out
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func applyTemplate(value string, vars map[string]string) string {
	result := value
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}

func applyTemplates(values []string, vars map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = applyTemplate(v, vars)
	}
	return out
}

func expandEnv(env map[string]string, vars map[string]string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = applyTemplate(v, vars)
	}
	return out
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func computeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
