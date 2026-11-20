// Package pinning hashes definitions with RFC 8785 canonical JSON and holds the function states.
package pinning

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/versatile-sh/connector-engine/port"
)

// Canonical returns the RFC 8785 (JCS) form of a JSON document.
func Canonical(raw []byte) ([]byte, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, port.Errorf(port.CodeInvalidArgument, "not JSON: %v", err)
	}
	if d.More() {
		return nil, port.Errorf(port.CodeInvalidArgument, "trailing data after the JSON value")
	}
	var b bytes.Buffer
	if err := write(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func write(b *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(x))
	case json.Number:
		f, err := strconv.ParseFloat(string(x), 64)
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return port.Errorf(port.CodeInvalidArgument, "number %s is not an IEEE 754 double", x)
		}
		b.WriteString(number(f))
	case string:
		quote(b, x)
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := write(b, e); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, func(a, c string) int { return slices.Compare(utf16.Encode([]rune(a)), utf16.Encode([]rune(c))) })
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			quote(b, k)
			b.WriteByte(':')
			if err := write(b, x[k]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	}
	return nil
}

func number(f float64) string {
	if f == 0 {
		return "0"
	}
	format := byte('e')
	if a := math.Abs(f); a >= 1e-6 && a < 1e21 {
		format = 'f'
	}
	s := strconv.FormatFloat(f, format, -1, 64)
	if i := strings.IndexByte(s, 'e'); i > 0 && s[i+2] == '0' {
		s = s[:i+2] + s[i+3:]
	}
	return s
}

func quote(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				b.WriteString(`\u00`)
				b.WriteString(hex.EncodeToString([]byte{byte(r)}))
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}

// Sum is the SHA-256 of bytes as a Hash.
func Sum(b []byte) port.Hash {
	s := sha256.Sum256(b)
	return port.Hash(hex.EncodeToString(s[:]))
}

// HashJSON is the SHA-256 of the canonical form of a JSON document.
func HashJSON(raw []byte) (port.Hash, error) {
	c, err := Canonical(raw)
	if err != nil {
		return "", err
	}
	return Sum(c), nil
}

func hashValue(v any) port.Hash {
	raw, _ := json.Marshal(v)
	h, _ := HashJSON(raw)
	return h
}

var definitionKeys = []string{"name", "title", "description", "inputSchema", "outputSchema", "annotations"}

var itemKeys = map[string][]string{
	port.ItemInstructions: {"instructions"},
	port.ItemPrompt:       {"name", "title", "description", "arguments"},
	port.ItemResource:     {"uri", "uriTemplate", "name", "title", "description", "mimeType", "annotations"},
}

func keysOf(name string) ([]string, bool) {
	if keys, ok := itemKeys[port.ItemKind(name)]; ok {
		return keys, true
	}
	return definitionKeys, false
}

// Hashed is the pinned part of a raw definition: for a tool the six keys and _meta ui.resourceUri and ui.visibility, for an item the keys of its kind.
func Hashed(name string, raw []byte) (map[string]json.RawMessage, error) {
	var all map[string]json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, port.Errorf(port.CodeInvalidArgument, "a definition is a JSON object: %v", err)
	}
	keys, item := keysOf(name)
	out := map[string]json.RawMessage{}
	for _, k := range keys {
		if v, ok := all[k]; ok {
			out[k] = v
		}
	}
	if item {
		return out, nil
	}
	var meta struct {
		UI map[string]json.RawMessage `json:"ui"`
	}
	if m, ok := all["_meta"]; ok && json.Unmarshal(m, &meta) == nil {
		ui := map[string]json.RawMessage{}
		for _, k := range []string{"resourceUri", "visibility"} {
			if v, ok := meta.UI[k]; ok {
				ui[k] = v
			}
		}
		if len(ui) > 0 {
			out["_meta"], _ = json.Marshal(map[string]any{"ui": ui})
		}
	}
	return out, nil
}

// DefinitionHash is the RFC 8785 hash of the pinned part of the raw definition of the function or item name; icons and order are excluded.
func DefinitionHash(name string, raw []byte) (port.Hash, error) {
	h, err := Hashed(name, raw)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(h)
	return HashJSON(b)
}

// ChangedFields lists the pinned keys whose canonical value differs between two raw definitions of the function or item name (connector.function.quarantined).
func ChangedFields(name string, oldRaw, newRaw []byte) ([]string, error) {
	a, err := Hashed(name, oldRaw)
	if err != nil {
		return nil, err
	}
	b, err := Hashed(name, newRaw)
	if err != nil {
		return nil, err
	}
	keys, item := keysOf(name)
	if !item {
		keys = append(slices.Clone(keys), "_meta")
	}
	var out []string
	for _, k := range keys {
		x, _ := canonicalOrNil(a[k])
		y, _ := canonicalOrNil(b[k])
		if !bytes.Equal(x, y) {
			out = append(out, k)
		}
	}
	return out, nil
}

func canonicalOrNil(raw json.RawMessage) ([]byte, error) {
	if raw == nil {
		return nil, nil
	}
	return Canonical(raw)
}

// InstructionsHash is the hash of the server instructions, pinned apart from the definitions.
func InstructionsHash(text string) port.Hash { return hashValue(text) }

// UIResourceHash is the hash of the content of one ui:// resource.
func UIResourceHash(content []byte) port.Hash { return Sum(content) }

// OverlayHash is the RFC 8785 hash of an overlay's title and description.
func OverlayHash(title, description string) port.Hash {
	return hashValue(map[string]string{"title": title, "description": description})
}

// CallHash is the RFC 8785 hash of {tenant, agent, connector, function, definition_hash, arguments}.
func CallHash(a port.Approval, arguments []byte) (port.Hash, error) {
	args := json.RawMessage("null")
	if len(arguments) > 0 {
		c, err := Canonical(arguments)
		if err != nil {
			return "", err
		}
		args = c
	}
	raw, _ := json.Marshal(map[string]any{
		"tenant": a.Tenant, "agent": a.Agent, "connector": a.Connector, "function": a.Function,
		"definition_hash": a.DefinitionHash, "arguments": args,
	})
	return HashJSON(raw)
}
