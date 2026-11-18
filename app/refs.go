package app

import (
	"fmt"
	"os"
	"strings"
)

func resolveRef(lookup func(string) string, ref string) (string, error) {
	kind, name, ok := strings.Cut(ref, ":")
	switch {
	case !ok || name == "":
		return "", fmt.Errorf("reference %q must be env:NAME or file:PATH", ref)
	case kind == "env":
		if v := lookup(name); v != "" {
			return v, nil
		}
		return "", fmt.Errorf("reference %q: the variable is empty", ref)
	case kind == "file":
		data, err := os.ReadFile(name)
		if err != nil {
			return "", fmt.Errorf("reference %q: %w", ref, err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return "", fmt.Errorf("reference %q must be env:NAME or file:PATH", ref)
}

func configString(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}
