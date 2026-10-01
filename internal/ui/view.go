package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Ero-Cat/Apk-Protector-Tool/internal/presets"
)

var (
	stTitle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00ADD8"))
	stSubtitle = lipgloss.NewStyle().Foreground(lipgloss.Color("#8B949E"))
	stStep     = lipgloss.NewStyle().Foreground(lipgloss.Color("#3DDC84"))
	stOK       = lipgloss.NewStyle().Foreground(lipgloss.Color("#3DDC84"))
	stWarn     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FEBC2E"))
	stErr      = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF7B72"))
	stLabel    = lipgloss.NewStyle().Foreground(lipgloss.Color("#79C0FF"))
	stCursor   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#3DDC84"))
	stDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("#6E7681"))
	stBox      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#2A3550")).Padding(0, 1)
	stJSON     = lipgloss.NewStyle().Foreground(lipgloss.Color("#C9D1D9"))
)

//nolint:ireturn // bubbletea requires the tea.Model interface
func (m model) View() string {
	switch m.screen {
	case screenWelcome:
		return m.viewWelcome()
	case screenProfile:
		return m.viewProfile()
	case screenForm:
		return m.viewForm()
	case screenReview:
		return m.viewReview()
	case screenRun:
		return m.viewRun()
	}
	return ""
}

func (m model) viewWelcome() string {
	var b strings.Builder
	b.WriteString(stTitle.Render("protector ui") + stSubtitle.Render(" — APK hardening wizard") + "\n\n")
	b.WriteString(stStep.Render("Step 1/5 · Select the APK") + "\n\n")
	b.WriteString("Path to the APK:\n")
	b.WriteString(m.input.View() + "\n\n")
	if m.bt.Zipalign != "" {
		b.WriteString(stDim.Render(fmt.Sprintf("detected build-tools: %s", m.bt.Zipalign)) + "\n")
	} else {
		b.WriteString(stWarn.Render("no Android build-tools detected — continue for scan-only steps, or install build-tools first") + "\n")
	}
	if m.inputErr != "" {
		b.WriteString("\n" + stErr.Render("✗ "+m.inputErr) + "\n")
	}
	b.WriteString("\n" + footer("enter continue · esc quit"))
	return b.String()
}

func (m model) viewProfile() string {
	defs := presets.Definitions()

	var b strings.Builder
	b.WriteString(stTitle.Render("protector ui") + stSubtitle.Render(" — APK hardening wizard") + "\n\n")
	b.WriteString(stStep.Render("Step 2/5 · Choose a profile") + "\n\n")

	for i, def := range defs {
		cursor := "  "
		if i == m.profileIdx {
			cursor = stCursor.Render("▸ ")
		}
		title := def.Title
		if i == m.profileIdx {
			title = stCursor.Render(def.Title)
		}
		b.WriteString(cursor + title + "\n")
	}

	sel := defs[m.profileIdx]
	b.WriteString("\n" + stLabel.Render(string(sel.Profile)) + stSubtitle.Render(" — "+sel.Desc) + "\n")
	b.WriteString(stLabel.Render("Steps: ") + strings.Join(sel.Steps, " → ") + "\n")

	b.WriteString("\n" + footer("↑↓/1-3 choose · enter confirm · esc back"))
	return b.String()
}

func (m model) viewForm() string {
	defs := presets.Definitions()

	var b strings.Builder
	title := "Step 3/5 · Configure"
	if m.profileIdx < len(defs) {
		title += stSubtitle.Render("  (profile: " + string(defs[m.profileIdx].Profile) + ")")
	}
	b.WriteString(stTitle.Render("protector ui") + "\n\n")
	b.WriteString(stStep.Render(title) + "\n\n")

	for i, spec := range m.fields {
		cursor := "  "
		if i == m.focus {
			cursor = stCursor.Render("▸ ")
		}
		if spec.Kind == KindToggle {
			marker := "[ ]"
			style := stDim
			if m.form.Toggles[spec.Key] {
				marker = "[x]"
				style = stOK
			}
			b.WriteString(cursor + style.Render(marker) + " " + spec.Label + "\n")
		} else {
			b.WriteString(cursor + stLabel.Render(spec.Label+":") + "\n")
			b.WriteString("    " + m.inputs[i].View() + "\n")
		}
		if i == m.focus && spec.Help != "" {
			b.WriteString("    " + stDim.Render(spec.Help) + "\n")
		}
	}

	if len(m.formErrs) > 0 {
		b.WriteString("\n" + stErr.Render("Fix the following before continuing:"))
		for _, e := range m.formErrs {
			b.WriteString("\n  " + stErr.Render("✗ "+e))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + footer("tab/↑↓ next field · space toggle · enter submit · esc back"))
	return b.String()
}

func (m model) viewReview() string {
	var b strings.Builder
	b.WriteString(stTitle.Render("protector ui") + "\n\n")
	b.WriteString(stStep.Render("Step 4/5 · Review") + "\n\n")

	for _, c := range m.checks {
		mark := stOK.Render("✓")
		switch c.Status {
		case CheckWarn:
			mark = stWarn.Render("!")
		case CheckInfo:
			mark = stDim.Render("·")
		}
		line := mark + " " + c.Label
		if c.Detail != "" {
			line += stDim.Render("  " + c.Detail)
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\n" + stLabel.Render("Configuration to write ("+m.cfgTarget+"):") + "\n")
	b.WriteString(stBox.Render(stJSON.Render(truncateLines(m.preview, 26))) + "\n")

	b.WriteString("\n" + footer("enter/y write config & run · w write only · e edit form · q quit"))
	return b.String()
}

func (m model) viewRun() string {
	var b strings.Builder
	b.WriteString(stTitle.Render("protector ui") + "\n\n")
	b.WriteString(stStep.Render("Step 5/5 · Running") + "\n\n")

	for _, stage := range m.stages {
		label := stage
		if l, ok := stageLabels[stage]; ok {
			label = l
		}
		b.WriteString(stOK.Render("✓ "+label) + "\n")
	}

	if !m.finished {
		b.WriteString(m.spinner.View() + stSubtitle.Render(" pipeline running…") + "\n")
		b.WriteString("\n" + footer("ctrl+c cancel"))
		return b.String()
	}

	if m.runErr != nil {
		b.WriteString("\n" + stErr.Render("✗ Failed: "+m.runErr.Error()) + "\n")
		if m.runOutput != "" {
			b.WriteString("\n" + stDim.Render("command output (tail):") + "\n")
			b.WriteString(stBox.Render(stDim.Render(truncateLines(m.runOutput, 12))) + "\n")
		}
		b.WriteString("\n" + footer("e back to form · q quit"))
		return b.String()
	}

	r := m.report
	b.WriteString("\n" + stOK.Render("✓ Done") + "\n\n")
	if r != nil {
		b.WriteString(stLabel.Render("Artifact:  ") + r.FinalAPK + "\n")
		if hash := r.Hashes["sha256"]; hash != "" {
			b.WriteString(stLabel.Render("SHA-256:   ") + stDim.Render(hash) + "\n")
		}
		var steps []string
		for _, s := range r.Steps {
			mark := stOK.Render("✓")
			if s.Status != "completed" {
				mark = stWarn.Render(s.Status)
			}
			steps = append(steps, mark+" "+s.Name)
		}
		b.WriteString(stLabel.Render("Steps:     ") + strings.Join(steps, "  ") + "\n")
		if r.Scan != nil {
			b.WriteString(stLabel.Render("Scan:      ") + fmt.Sprintf("%d anti-env findings, %d key leaks",
				len(r.Scan.AntiEnvironment), len(r.Scan.KeyLeaks)) + "\n")
		}
		b.WriteString(stLabel.Render("Duration:  ") + r.Duration + "\n")
	}
	if m.form != nil && m.form.Values[keyReport] != "" {
		b.WriteString(stLabel.Render("Report:    ") + m.form.Values[keyReport] + "\n")
	}
	b.WriteString("\n" + footer("q quit"))
	return b.String()
}

func footer(keys string) string {
	return stDim.Render(keys)
}

// truncateLines keeps at most n lines, marking the cut.
func truncateLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n") + "\n…"
}
