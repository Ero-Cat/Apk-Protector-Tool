package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// State remembers non-sensitive wizard choices between runs (paths, alias
// names — never passwords or secrets).
type State struct {
	LastInput     string `json:"last_input,omitempty"`
	LastOutput    string `json:"last_output,omitempty"`
	ZipalignPath  string `json:"zipalign_path,omitempty"`
	ApksignerPath string `json:"apksigner_path,omitempty"`
	KeytoolPath   string `json:"keytool_path,omitempty"`
	Keystore      string `json:"keystore,omitempty"`
	KeyAlias      string `json:"key_alias,omitempty"`
	ReportPath    string `json:"report_path,omitempty"`
}

// statePath returns the default state file location:
// <os.UserConfigDir>/protector/state.json.
func statePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "protector", "state.json"), nil
}

// LoadState reads the persisted state; it returns an empty state on any
// error (a missing state file is not a problem).
func LoadState() *State {
	path, err := statePath()
	if err != nil {
		return &State{}
	}
	st, err := loadStateFrom(path)
	if err != nil {
		return &State{}
	}
	return st
}

// Save persists the state to its default location; errors are returned to
// the caller (the wizard only warns).
func (s *State) Save() error {
	path, err := statePath()
	if err != nil {
		return err
	}
	return s.SaveTo(path)
}

// SaveTo writes the state as JSON to an explicit path.
func (s *State) SaveTo(path string) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func loadStateFrom(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	st := &State{}
	if err := json.Unmarshal(data, st); err != nil {
		return nil, err
	}
	return st, nil
}
