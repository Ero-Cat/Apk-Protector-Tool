// Package envref expands ${VAR} references in configuration values.
//
// Only the ${...} form is recognized: bare $VAR is left untouched so that
// values merely starting with '$' survive. A reference to an unset variable
// without a default is an error so that missing secrets fail fast instead of
// being used as literal text.
package envref

import (
	"fmt"
	"os"
	"strings"
)

// Expand replaces ${VAR} and ${VAR:-default} references in s with the
// corresponding environment values.
func Expand(s string) (string, error) {
	if !strings.Contains(s, "${") {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '$' || i+1 >= len(s) || s[i+1] != '{' {
			b.WriteByte(s[i])
			i++
			continue
		}
		end := strings.IndexByte(s[i+2:], '}')
		if end < 0 {
			// No closing brace: keep the remainder as literal text.
			b.WriteString(s[i:])
			break
		}
		body := s[i+2 : i+2+end]
		i = i + 2 + end + 1

		name, def, hasDefault := strings.Cut(body, ":-")
		if !validName(name) {
			return "", fmt.Errorf("invalid environment variable reference ${%s}", body)
		}
		value, ok := os.LookupEnv(name)
		if !ok || value == "" {
			if !hasDefault {
				return "", fmt.Errorf("environment variable %q is not set (referenced as ${%s})", name, body)
			}
			value = def
		}
		b.WriteString(value)
	}
	return b.String(), nil
}

// validName reports whether name is a usable environment variable name.
func validName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
		case c >= '0' && c <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
