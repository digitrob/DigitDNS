package migrate

import (
	"fmt"

	"github.com/BurntSushi/toml"
	"go.yaml.in/yaml/v3"
)

// DecodeYAML reads an AdGuard Home YAML document into the generic tree Redact
// and the source parsers inspect. It is deliberately not a typed decode: an
// unknown field must not make the whole import fail, and a parser should see the
// exact key it is about to copy.
func DecodeYAML(data []byte) (map[string]any, error) {
	var out map[string]any
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode YAML: %w", err)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

// DecodeTOML reads a Pi-hole pihole.toml document into the same generic tree.
func DecodeTOML(data []byte) (map[string]any, error) {
	var out map[string]any
	if err := toml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode TOML: %w", err)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}
