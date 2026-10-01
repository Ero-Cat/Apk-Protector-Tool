package ui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Ero-Cat/Apk-Protector-Tool/internal/app"
	"github.com/Ero-Cat/Apk-Protector-Tool/internal/presets"
)

// defaultConfigTarget is where the review screen writes the generated
// configuration.
const defaultConfigTarget = "protector.config.json"

// runTimeout bounds a wizard-triggered pipeline run.
const runTimeout = 30 * time.Minute

type screen int

const (
	screenWelcome screen = iota
	screenProfile
	screenForm
	screenReview
	screenRun
)

type runEventKind int

const (
	evStage runEventKind = iota
	evDone
	evFailed
)

// runEvent is delivered from the pipeline goroutine back into the UI loop.
type runEvent struct {
	kind   runEventKind
	stage  string
	report *app.Report
	err    error
	output string
}

var stageLabels = map[string]string{
	"scan":      "Security scan",
	"protect":   "Protections",
	"reinforce": "Third-party hardener",
	"align":     "Zipalign",
	"sign":      "Signing",
	"verify":    "Verification",
	"finalize":  "Finalize",
}

// Run starts the interactive wizard. It refuses to run without a terminal
// and points users at the headless -profile mode instead.
func Run() error {
	if os.Getenv("TERM") == "dumb" || !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		fmt.Fprintln(os.Stderr, "protector ui: an interactive terminal is required.")
		fmt.Fprintln(os.Stderr, "Headless alternative: protector -profile quick|full|sign-only -input app.apk ... (see protector -h)")
		os.Exit(2)
	}
	program := tea.NewProgram(newModel(), tea.WithAltScreen())
	final, err := program.Run()
	if err != nil {
		return err
	}
	if fm, ok := final.(model); ok && fm.quitMsg != "" {
		fmt.Println(fm.quitMsg)
	}
	return nil
}

type model struct {
	state *State
	bt    BuildTools

	width  int
	height int
	screen screen

	quitMsg string // printed after the program exits

	// S1
	input      textinput.Model
	inputErr   string
	pendingInp string

	// S2
	profileIdx int

	// S3
	form     *Form
	fields   []FieldSpec
	inputs   []textinput.Model
	hasInput []bool
	focus    int
	formErrs []string

	// S4
	preview   string
	checks    []CheckLine
	cfgTarget string

	// S5
	spinner   spinner.Model
	events    <-chan runEvent
	cancel    context.CancelFunc
	stages    []string
	report    *app.Report
	runErr    error
	runOutput string
	finished  bool
}

func newModel() model {
	st := LoadState()
	bt := DetectBuildTools()

	input := textinput.New()
	input.Placeholder = "path/to/app-release.apk"
	if st.LastInput != "" {
		input.SetValue(st.LastInput)
	}
	input.CharLimit = 4096
	input.Width = 64
	input.Focus()

	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8"))))

	return model{
		state:     st,
		bt:        bt,
		input:     input,
		spinner:   sp,
		cfgTarget: defaultConfigTarget,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

//nolint:ireturn // bubbletea requires the tea.Model interface
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case runEvent:
		return m.handleRunEvent(msg)
	case spinner.TickMsg:
		if m.screen == screenRun && !m.finished {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	case tea.KeyMsg:
		switch m.screen {
		case screenWelcome:
			return m.updateWelcome(msg)
		case screenProfile:
			return m.updateProfile(msg)
		case screenForm:
			return m.updateForm(msg)
		case screenReview:
			return m.updateReview(msg)
		case screenRun:
			return m.updateRunKeys(msg)
		}
	}
	return m, nil
}

func (m model) updateWelcome(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit
	case "enter":
		path := expandTilde(strings.TrimSpace(m.input.Value()))
		m.input.SetValue(path)
		m.inputErr = ""
		if path == "" {
			m.inputErr = "enter the path to the APK you want to harden"
			return m, nil
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			m.inputErr = fmt.Sprintf("%s is not a readable file", path)
			return m, nil
		}
		if !looksLikeAPK(path) {
			m.inputErr = fmt.Sprintf("%s does not look like an APK (missing zip magic bytes)", path)
			return m, nil
		}
		m.pendingInp = path
		m.screen = screenProfile
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateProfile(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	defs := presets.Definitions()
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc":
		m.screen = screenWelcome
		return m, nil
	case "up", "k":
		if m.profileIdx > 0 {
			m.profileIdx--
		}
	case "down", "j":
		if m.profileIdx < len(defs)-1 {
			m.profileIdx++
		}
	case "1", "2", "3":
		if idx := int(msg.String()[0] - '1'); idx >= 0 && idx < len(defs) {
			m.profileIdx = idx
		}
	case "enter":
		m.buildForm(defs[m.profileIdx].Profile, m.pendingInp)
		m.screen = screenForm
		return m, m.focusCmd()
	}
	return m, nil
}

func (m model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	spec := m.fields[m.focus]

	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.screen = screenProfile
		return m, nil
	case "tab", "down":
		m.moveFocus(1)
		return m, m.focusCmd()
	case "shift+tab", "up":
		m.moveFocus(-1)
		return m, m.focusCmd()
	case "enter", " ":
		if spec.Kind == KindToggle {
			m.form.Toggles[spec.Key] = !m.form.Toggles[spec.Key]
			return m, nil
		}
		if key == " " {
			break // space is regular input content
		}
		if m.focus < len(m.fields)-1 {
			m.moveFocus(1)
			return m, m.focusCmd()
		}
		return m.submitForm()
	}

	if m.hasInput[m.focus] {
		var cmd tea.Cmd
		m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) submitForm() (tea.Model, tea.Cmd) {
	m.syncInputsToForm()
	if errs := m.form.Validate(); len(errs) > 0 {
		m.formErrs = errs
		return m, nil
	}
	m.formErrs = nil
	preview, err := m.form.PreviewJSON()
	if err != nil {
		m.formErrs = []string{err.Error()}
		return m, nil
	}
	m.preview = preview
	m.checks = m.form.ReviewChecks()
	m.screen = screenReview
	return m, nil
}

func (m model) updateReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc", "e", "b":
		m.screen = screenForm
		return m, m.focusCmd()
	case "y", "enter":
		if err := m.writeConfig(); err != nil {
			m.formErrs = []string{fmt.Sprintf("write %s: %v", m.cfgTarget, err)}
			m.screen = screenForm
			return m, nil
		}
		return m.beginRun()
	case "w":
		if err := m.writeConfig(); err != nil {
			m.formErrs = []string{fmt.Sprintf("write %s: %v", m.cfgTarget, err)}
			m.screen = screenForm
			return m, nil
		}
		m.quitMsg = "Configuration written to " + m.cfgTarget
		return m, tea.Quit
	}
	return m, nil
}

func (m model) updateRunKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.cancel != nil && !m.finished {
			m.cancel()
		}
		return m, tea.Quit
	case "q", "esc":
		if m.finished {
			return m, tea.Quit
		}
	case "e":
		if m.finished {
			m.screen = screenForm
			return m, m.focusCmd()
		}
	}
	return m, nil
}

func (m model) handleRunEvent(ev runEvent) (tea.Model, tea.Cmd) {
	switch ev.kind {
	case evStage:
		m.stages = append(m.stages, ev.stage)
		return m, waitEvent(m.events)
	case evDone:
		m.report = ev.report
		m.finished = true
		m.saveState()
		return m, nil
	case evFailed:
		m.runErr = ev.err
		m.runOutput = ev.output
		m.finished = true
		m.saveState()
		return m, nil
	}
	return m, nil
}

// buildForm assembles the S3 form for a profile, prefilled from persisted
// state, autodetection and conventional environment variable names.
func (m *model) buildForm(profile presets.Profile, input string) {
	m.form = NewForm(profile, input)
	m.fields = m.form.Fields()
	m.inputs = make([]textinput.Model, len(m.fields))
	m.hasInput = make([]bool, len(m.fields))

	prefill := map[string]string{
		keyZipalign:  firstNonEmpty(m.state.ZipalignPath, m.bt.Zipalign),
		keyApksigner: firstNonEmpty(m.state.ApksignerPath, m.bt.Apksigner),
		keyKeytool:   firstNonEmpty(m.state.KeytoolPath, m.bt.Keytool),
		keyKeystore:  m.state.Keystore,
		keyAlias:     m.state.KeyAlias,
		keyOutput:    m.state.LastOutput,
		keyReport:    m.state.ReportPath,
	}
	for env, key := range map[string]string{
		"APK_STORE_PASS":     keyStorePassEnv,
		"APK_KEY_PASS":       keyKeyPassEnv,
		"APK_PROTECT_SECRET": keySecretEnv,
	} {
		if os.Getenv(env) != "" && prefill[key] == "" {
			prefill[key] = env
		}
	}

	for i, spec := range m.fields {
		if spec.Kind == KindToggle {
			continue
		}
		ti := textinput.New()
		ti.CharLimit = 4096
		ti.Width = 56
		ti.Placeholder = spec.Placeholder
		if v := prefill[spec.Key]; v != "" {
			ti.SetValue(v)
		}
		m.inputs[i] = ti
		m.hasInput[i] = true
	}
	m.focus = 0
	m.formErrs = nil
}

func (m *model) moveFocus(delta int) {
	m.focus += delta
	if m.focus < 0 {
		m.focus = len(m.fields) - 1
	}
	if m.focus >= len(m.fields) {
		m.focus = 0
	}
}

func (m *model) focusCmd() tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.inputs {
		if m.hasInput[i] {
			if i == m.focus {
				cmds = append(cmds, m.inputs[i].Focus())
			} else {
				m.inputs[i].Blur()
			}
		}
	}
	return tea.Batch(cmds...)
}

func (m *model) syncInputsToForm() {
	for i, spec := range m.fields {
		if m.hasInput[i] {
			m.form.Values[spec.Key] = m.inputs[i].Value()
		}
	}
}

func (m model) writeConfig() error {
	return os.WriteFile(m.cfgTarget, []byte(m.preview+"\n"), 0o644)
}

func (m model) beginRun() (tea.Model, tea.Cmd) {
	m.screen = screenRun
	m.stages = nil
	m.report = nil
	m.runErr = nil
	m.runOutput = ""
	m.finished = false

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	m.cancel = cancel

	ch := make(chan runEvent, 32)
	m.events = ch

	buf := newSyncBuffer()
	form := m.form
	go executePipeline(ctx, form, ch, buf)

	return m, tea.Batch(m.spinner.Tick, waitEvent(ch))
}

func waitEvent(ch <-chan runEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return ev
	}
}

// executePipeline runs the hardening pipeline and streams progress back over
// ch. It closes ch when done.
func executePipeline(ctx context.Context, form *Form, ch chan<- runEvent, buf *syncBuffer) {
	defer close(ch)

	cfg, err := form.ToConfig()
	if err != nil {
		ch <- runEvent{kind: evFailed, err: err}
		return
	}

	tool := app.NewTool(cfg)
	tool.Stdout = buf
	tool.Stderr = buf
	tool.Progress = func(stage string) {
		ch <- runEvent{kind: evStage, stage: stage}
	}

	report, err := tool.Run(ctx)
	if err != nil {
		ch <- runEvent{kind: evFailed, err: err, output: buf.tail(15)}
		return
	}
	if cfg.Reporting.JSON != "" {
		if werr := app.WriteReport(cfg.Reporting.JSON, report); werr != nil {
			ch <- runEvent{kind: evFailed, err: fmt.Errorf("write report: %w", werr)}
			return
		}
	}
	ch <- runEvent{kind: evDone, report: report}
}

func (m *model) saveState() {
	if m.form == nil {
		return
	}
	m.state.LastInput = m.form.InputAPK
	m.state.LastOutput = m.form.Values[keyOutput]
	m.state.ZipalignPath = m.form.Values[keyZipalign]
	m.state.ApksignerPath = m.form.Values[keyApksigner]
	m.state.KeytoolPath = m.form.Values[keyKeytool]
	m.state.Keystore = m.form.Values[keyKeystore]
	m.state.KeyAlias = m.form.Values[keyAlias]
	m.state.ReportPath = m.form.Values[keyReport]
	_ = m.state.Save()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// syncBuffer collects child process output concurrently.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func newSyncBuffer() *syncBuffer { return &syncBuffer{} }

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// tail returns the last n lines written to the buffer.
func (b *syncBuffer) tail(n int) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	lines := strings.Split(strings.TrimRight(b.buf.String(), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
