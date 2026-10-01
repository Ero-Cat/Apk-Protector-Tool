package envref

import (
	"strings"
	"testing"
)

func TestExpand(t *testing.T) {
	t.Setenv("ENVREF_A", "alpha")
	t.Setenv("ENVREF_EMPTY", "")

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "no refs", in: "plain value", want: "plain value"},
		{name: "simple ref", in: "${ENVREF_A}", want: "alpha"},
		{name: "embedded ref", in: "pre-${ENVREF_A}-post", want: "pre-alpha-post"},
		{name: "multiple refs", in: "${ENVREF_A}/${ENVREF_A}", want: "alpha/alpha"},
		{name: "default used when unset", in: "${ENVREF_MISSING:-fallback}", want: "fallback"},
		{name: "default used when empty", in: "${ENVREF_EMPTY:-fallback}", want: "fallback"},
		{name: "value wins over default", in: "${ENVREF_A:-fallback}", want: "alpha"},
		{name: "empty default allowed", in: "${ENVREF_MISSING:-}", want: ""},
		{name: "unset without default errors", in: "${ENVREF_MISSING}", wantErr: true},
		{name: "invalid name errors", in: "${1BAD}", wantErr: true},
		{name: "invalid name with default errors", in: "${bad-name:-x}", wantErr: true},
		{name: "bare dollar untouched", in: "$HOME and $PATH", want: "$HOME and $PATH"},
		{name: "unclosed brace literal", in: "${ENVREF_A", want: "${ENVREF_A"},
		{name: "dollar brace at end", in: "x${", want: "x${"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Expand(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Expand(%q) = %q, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Expand(%q) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("Expand(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestExpandErrorNamesVariable(t *testing.T) {
	_, err := Expand("secret is ${ENVREF_NOPE} here")
	if err == nil {
		t.Fatal("want error for unset variable")
	}
	if !strings.Contains(err.Error(), "ENVREF_NOPE") {
		t.Fatalf("error should name the missing variable, got: %v", err)
	}
}
