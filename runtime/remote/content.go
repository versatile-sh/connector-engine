package remote

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	deadlines "github.com/versatile-sh/connector-engine/internal/deadline"
	"github.com/versatile-sh/connector-engine/port"
)

var _ port.ContentRuntime = (*Runtime)(nil)

func collect[T any](ctx context.Context, out *[]port.RawDefinition, method string, list iter.Seq2[T, error], name func(T) string) error {
	for v, err := range list {
		var rpc *jsonrpc.Error
		if errors.As(err, &rpc) && rpc.Code == jsonrpc.CodeMethodNotFound {
			return nil
		}
		if err != nil {
			return port.Errorf(port.CodeUnavailable, "%s: %v", method, err)
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return port.Errorf(port.CodeInternal, "%s: %v", method, err)
		}
		*out = append(*out, port.RawDefinition{Name: name(v), Raw: raw})
	}
	return ctx.Err()
}

// ListContent lists the prompts, resources and resource templates the server declares, named prompt:<name> and res:<uri or uri template>.
func (r *Runtime) ListContent(ctx context.Context, h port.Handle) ([]port.RawDefinition, error) {
	in, err := r.started.Get(h)
	if err != nil {
		return nil, err
	}
	caps := in.session.InitializeResult().Capabilities
	var out []port.RawDefinition
	if caps != nil && caps.Prompts != nil {
		if err := collect(ctx, &out, "prompts/list", in.session.Prompts(ctx, nil), func(p *mcp.Prompt) string { return port.ItemPrompt + p.Name }); err != nil {
			return nil, err
		}
	}
	if caps != nil && caps.Resources != nil {
		if err := collect(ctx, &out, "resources/list", in.session.Resources(ctx, nil), func(r *mcp.Resource) string { return port.ItemResource + r.URI }); err != nil {
			return nil, err
		}
		if err := collect(ctx, &out, "resources/templates/list", in.session.ResourceTemplates(ctx, nil), func(t *mcp.ResourceTemplate) string { return port.ItemResource + t.URITemplate }); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Content gets a prompt with {"arguments": {...}} or reads a resource with {"uri": ...}, and returns the messages or contents as the result's content.
func (r *Runtime) Content(ctx context.Context, h port.Handle, item string, arguments json.RawMessage, deadline time.Time) (port.Result, error) {
	in, err := r.started.Get(h)
	if err != nil {
		return port.Result{}, err
	}
	ctx, cancel := deadlines.Apply(ctx, deadline)
	defer cancel()
	var a struct {
		Arguments map[string]string `json:"arguments"`
		URI       string            `json:"uri"`
	}
	if err := json.Unmarshal(arguments, &a); err != nil {
		return port.Result{}, port.Errorf(port.CodeInvalidArgument, "item arguments: %v", err)
	}
	var content any
	switch kind := port.ItemKind(item); kind {
	case port.ItemPrompt:
		name := strings.TrimPrefix(item, kind)
		res, err := in.session.GetPrompt(ctx, &mcp.GetPromptParams{Name: name, Arguments: a.Arguments})
		if err != nil {
			return port.Result{}, in.failed(ctx, err, "prompts/get", name)
		}
		content = res.Messages
	case port.ItemResource:
		res, err := in.session.ReadResource(ctx, &mcp.ReadResourceParams{URI: a.URI})
		if err != nil {
			return port.Result{}, in.failed(ctx, err, "resources/read", a.URI)
		}
		content = res.Contents
	default:
		return port.Result{}, port.Errorf(port.CodeInvalidArgument, "%s is not a prompt or a resource", item)
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return port.Result{}, port.Errorf(port.CodeInternal, "content: %v", err)
	}
	return port.Result{Content: raw}, nil
}

// Complete asks completion/complete for an argument of a prompt or resource template item; request carries argument and context as MCP names them.
func (r *Runtime) Complete(ctx context.Context, h port.Handle, item string, request json.RawMessage) (json.RawMessage, error) {
	in, err := r.started.Get(h)
	if err != nil {
		return nil, err
	}
	var p mcp.CompleteParams
	if err := json.Unmarshal(request, &p); err != nil {
		return nil, port.Errorf(port.CodeInvalidArgument, "completion request: %v", err)
	}
	kind := port.ItemKind(item)
	p.Ref = &mcp.CompleteReference{Type: "ref/resource", URI: strings.TrimPrefix(item, kind)}
	if kind == port.ItemPrompt {
		p.Ref = &mcp.CompleteReference{Type: "ref/prompt", Name: strings.TrimPrefix(item, kind)}
	}
	res, err := in.session.Complete(ctx, &p)
	if err != nil {
		return nil, in.failed(ctx, err, "completion/complete", item)
	}
	return json.Marshal(res.Completion)
}
