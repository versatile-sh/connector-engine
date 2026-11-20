package console

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/agents-control/connector-engine/core/names"
	"github.com/agents-control/connector-engine/port"
)

type separatorCache struct {
	mu      sync.Mutex
	learned string
}

func (s *separatorCache) get() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.learned
}

func (s *separatorCache) set(sep string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.learned = sep
}

type exposedFunction struct {
	Name      string           `json:"name"`
	Connector port.ConnectorID `json:"connector"`
	Function  string           `json:"function"`
}

func (c *Console) separator(ctx context.Context, agent string) string {
	if sep := c.sep.get(); sep != "" {
		return sep
	}
	if sep := c.learnSeparator(ctx, agent); sep != "" {
		c.sep.set(sep)
		return sep
	}
	return names.Separator
}

func (c *Console) learnSeparator(ctx context.Context, agent string) string {
	if agent == "" {
		return ""
	}
	conns, err := c.Engine.connectors(ctx)
	if err != nil {
		return ""
	}
	alias := map[port.ConnectorID]string{}
	for _, v := range conns {
		alias[v.Connector.ID] = v.Connector.Alias
	}
	var list struct {
		Functions []exposedFunction `json:"functions"`
	}
	if c.Engine.Call(ctx, http.MethodGet, "/v1/agents/"+esc(agent)+"/functions"+q("workspace", c.Engine.Workspace), nil, &list) != nil {
		return ""
	}
	for _, f := range list.Functions {
		a := alias[f.Connector]
		if a != "" && strings.HasPrefix(f.Name, a) && strings.HasSuffix(f.Name, f.Function) && len(f.Name) > len(a)+len(f.Function) {
			return f.Name[len(a) : len(f.Name)-len(f.Function)]
		}
	}
	return ""
}
