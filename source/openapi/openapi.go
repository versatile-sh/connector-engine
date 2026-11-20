// Package openapi is the ListingSource that turns OpenAPI 3.x documents, from a directory or addresses, into rest_declarative listings.
package openapi

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/internal/egress"
	"github.com/versatile-sh/connector-engine/internal/fetch"
	"github.com/versatile-sh/connector-engine/internal/importer"
	"github.com/versatile-sh/connector-engine/port"
)

// Registry is the name recorded as the provenance of an OpenAPI import.
const Registry = "openapi"

var (
	methods   = []string{"get", "post", "put", "patch", "delete"}
	unsafe    = regexp.MustCompile(`[^A-Za-z0-9_-]+`)
	unsafeDNS = regexp.MustCompile(`[^a-z0-9._-]+`)
	maxDepth  = 12
)

// Config names where the documents are; any of the two may be empty.
type Config struct {
	Dir   string
	URLs  []string
	HTTP  *http.Client
	Guard egress.Guard
	Now   func() time.Time
}

// New returns a source over the documents of cfg, directory files in name order and then the addresses in order.
func New(cfg Config) *importer.Static {
	if cfg.HTTP == nil {
		cfg.HTTP = cfg.Guard.Client()
	}
	return &importer.Static{Registry: Registry, Now: cfg.Now, Load: func(ctx context.Context) ([]port.Manifest, error) { return load(ctx, cfg) }}
}

func load(ctx context.Context, cfg Config) ([]port.Manifest, error) {
	docs := map[string][]byte{}
	var order []string
	if cfg.Dir != "" {
		entries, err := os.ReadDir(cfg.Dir)
		if err != nil {
			return nil, port.Errorf(port.CodeNotFound, "openapi directory: %v", err)
		}
		for _, e := range entries {
			if e.IsDir() || !slices.Contains([]string{".json", ".yaml", ".yml"}, strings.ToLower(filepath.Ext(e.Name()))) {
				continue
			}
			b, err := os.ReadFile(filepath.Join(cfg.Dir, e.Name()))
			if err != nil {
				return nil, port.Errorf(port.CodeUnavailable, "%s: %v", e.Name(), err)
			}
			docs[e.Name()] = b
			order = append(order, e.Name())
		}
	}
	for _, u := range cfg.URLs {
		b, err := fetch.Do(ctx, cfg.HTTP, http.MethodGet, u, nil, nil)
		if err != nil {
			return nil, err
		}
		docs[u] = b
		order = append(order, u)
	}
	var out []port.Manifest
	for _, origin := range order {
		m, err := manifest(docs[origin])
		if err != nil {
			return nil, port.Errorf(port.CodeInvalidManifest, "%s: %v", origin, err)
		}
		out = append(out, m)
	}
	return out, nil
}

type node = map[string]any

func manifest(data []byte) (port.Manifest, error) {
	raw, err := importer.YAMLToJSON(data)
	if err != nil {
		return port.Manifest{}, err
	}
	var doc node
	if err := json.Unmarshal(raw, &doc); err != nil {
		return port.Manifest{}, err
	}
	if v, _ := doc["openapi"].(string); !strings.HasPrefix(v, "3.") {
		return port.Manifest{}, port.Errorf(port.CodeInvalidManifest, "not an OpenAPI 3.x document")
	}
	info := obj(doc["info"])
	base, err := baseURL(doc)
	if err != nil {
		return port.Manifest{}, err
	}
	title := str(info["title"])
	m := port.Manifest{
		Name:        reverseHost(base.Hostname()) + "/" + unsafeDNS.ReplaceAllString(strings.ToLower(title), "-"),
		Version:     str(info["version"]),
		Title:       title,
		Description: str(info["description"]),
		Kind:        port.KindRESTDeclarative,
		Publisher:   port.Publisher{Name: first(str(obj(info["contact"])["name"]), base.Hostname())},
		Licence:     str(obj(info["license"])["identifier"]),
		Egress:      &port.Egress{AllowHosts: []string{base.Host}},
		REST:        &port.REST{BaseURL: base.String()},
	}
	paths := obj(doc["paths"])
	used := map[string]bool{}
	for _, path := range slices.Sorted(maps.Keys(paths)) {
		item := obj(paths[path])
		for _, method := range methods {
			op, ok := item[method].(node)
			if !ok {
				continue
			}
			if f, ok := function(doc, item, op, method, path, used); ok {
				m.REST.Functions = append(m.REST.Functions, f)
			}
		}
	}
	return m, nil
}

func baseURL(doc node) (*url.URL, error) {
	servers, _ := doc["servers"].([]any)
	if len(servers) == 0 {
		return nil, port.Errorf(port.CodeInvalidManifest, "servers[0].url is required")
	}
	server := obj(servers[0])
	raw := str(server["url"])
	for name, v := range obj(server["variables"]) {
		raw = strings.ReplaceAll(raw, "{"+name+"}", str(obj(v)["default"]))
	}
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil || u.Host == "" {
		return nil, port.Errorf(port.CodeInvalidManifest, "servers[0].url %q is not an absolute address", raw)
	}
	return u, nil
}

func function(doc, item, op node, method, path string, used map[string]bool) (port.RESTFunction, bool) {
	schema := node{"type": "object", "properties": node{}}
	props, required := schema["properties"].(node), []any{}
	params := append(list(item["parameters"]), list(op["parameters"])...)
	for _, raw := range params {
		p := obj(resolve(doc, raw, nil, 0))
		in, name := str(p["in"]), str(p["name"])
		if name == "" || in != "path" && in != "query" {
			continue
		}
		prop := obj(resolve(doc, p["schema"], nil, 0))
		if d := str(p["description"]); d != "" {
			prop["description"] = d
		}
		if in == "query" {
			prop["x-in"] = "query"
		}
		props[name] = prop
		if in == "path" || p["required"] == true {
			required = append(required, name)
		}
	}
	if body, present := op["requestBody"]; present {
		b := obj(obj(obj(resolve(doc, body, nil, 0))["content"])["application/json"])
		bs := obj(resolve(doc, b["schema"], nil, 0))
		if str(bs["type"]) != "object" && bs["properties"] == nil {
			return port.RESTFunction{}, false
		}
		for k, v := range obj(bs["properties"]) {
			if _, taken := props[k]; !taken {
				props[k] = v
			}
		}
		required = append(required, list(bs["required"])...)
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	in, _ := json.Marshal(schema)
	f := port.RESTFunction{
		Name:        functionName(op, method, path, used),
		Method:      strings.ToUpper(method),
		Path:        path,
		Description: strings.TrimSpace(strings.Join(nonEmpty(str(op["summary"]), str(op["description"])), "\n\n")),
		InputSchema: in,
	}
	if out := responseSchema(doc, op); out != nil {
		f.OutputSchema = out
	}
	return f, true
}

func responseSchema(doc, op node) json.RawMessage {
	responses := obj(op["responses"])
	for _, code := range []string{"200", "201", "2XX", "default"} {
		r, ok := responses[code]
		if !ok {
			continue
		}
		s := obj(resolve(doc, obj(obj(obj(resolve(doc, r, nil, 0))["content"])["application/json"])["schema"], nil, 0))
		if str(s["type"]) == "object" {
			b, _ := json.Marshal(s)
			return b
		}
		return nil
	}
	return nil
}

func functionName(op node, method, path string, used map[string]bool) string {
	n := strings.Trim(unsafe.ReplaceAllString(str(op["operationId"]), "_"), "_")
	if n == "" {
		n = method + "_" + strings.Trim(unsafe.ReplaceAllString(path, "_"), "_")
	}
	n = n[:min(len(n), 60)]
	for i, base := 2, n; used[n]; i++ {
		n = base + "_" + strconv.Itoa(i)
	}
	used[n] = true
	return n
}

func resolve(root node, v any, stack []string, depth int) any {
	if depth > maxDepth {
		return node{}
	}
	switch x := v.(type) {
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = resolve(root, e, stack, depth+1)
		}
		return out
	case node:
		if ref, ok := x["$ref"].(string); ok {
			if slices.Contains(stack, ref) {
				return node{"type": "object"}
			}
			target, ok := lookup(root, ref)
			if !ok {
				return node{}
			}
			return resolve(root, target, append(slices.Clone(stack), ref), depth+1)
		}
		out := node{}
		for k, e := range x {
			if k == "example" || k == "examples" || k == "xml" {
				continue
			}
			out[k] = resolve(root, e, stack, depth+1)
		}
		return out
	}
	return v
}

func lookup(root node, ref string) (any, bool) {
	rest, ok := strings.CutPrefix(ref, "#/")
	if !ok {
		return nil, false
	}
	var cur any = root
	for _, part := range strings.Split(rest, "/") {
		part = strings.NewReplacer("~1", "/", "~0", "~").Replace(part)
		n, ok := cur.(node)
		if !ok {
			return nil, false
		}
		if cur, ok = n[part]; !ok {
			return nil, false
		}
	}
	return cur, true
}

func reverseHost(host string) string {
	labels := strings.Split(host, ".")
	slices.Reverse(labels)
	return strings.Join(labels, ".")
}

func obj(v any) node {
	if n, ok := v.(node); ok {
		return n
	}
	return node{}
}

func list(v any) []any {
	l, _ := v.([]any)
	return l
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func nonEmpty(ss ...string) []string {
	return slices.DeleteFunc(ss, func(s string) bool { return s == "" })
}
