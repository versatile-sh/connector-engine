package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/versatile-sh/connector-engine/internal/grpctest"
	plugout "github.com/versatile-sh/connector-engine/internal/plugkit/outbox"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/service"
	"github.com/versatile-sh/connector-engine/store/storetest"
)

const workspace = "w1"

type items[T any] struct {
	Items []T `json:"items"`
}

type cloudEvent struct {
	ID          string          `json:"id"`
	SpecVersion string          `json:"specversion"`
	Type        string          `json:"type"`
	Subject     string          `json:"subject"`
	Tenant      string          `json:"tenant"`
	Data        json.RawMessage `json:"data"`
}

type sinkReceiver struct {
	*httptest.Server
	mu       sync.Mutex
	events   []cloudEvent
	rejected []string
}

func newSink(t *testing.T) *sinkReceiver {
	t.Helper()
	key, err := plugout.ParseKey(sinkSecret)
	if err != nil {
		t.Fatal(err)
	}
	s := &sinkReceiver{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := plugout.Verify(key, r.Header, body, time.Now(), 5*time.Minute); err != nil {
			s.rejected = append(s.rejected, err.Error())
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var ev cloudEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			s.rejected = append(s.rejected, err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		s.events = append(s.events, ev)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *sinkReceiver) eventsFor(tenant string) []cloudEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []cloudEvent
	for _, e := range s.events {
		if e.Tenant == tenant {
			out = append(out, e)
		}
	}
	return out
}

func (s *sinkReceiver) failures() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.rejected)
}

type world struct {
	t       *testing.T
	bin     binaries
	cfg     settings
	eng     *engine
	sink    *sinkReceiver
	mcp     *mcpUpstream
	drift   *mcpUpstream
	rest    *restUpstream
	grpc    *grpctest.Server
	hasPG   bool
	tenants map[string]bool
}

func waitFor(t *testing.T, what string, limit time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (w *world) operator() caller {
	return caller{bin: w.bin, url: w.eng.Operator, token: operatorToken}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func remoteListing(name, version, url string) map[string]any {
	return map[string]any{"name": name, "version": version, "kind": "mcp_remote", "publisher": map[string]any{"name": "Acme"},
		"remote": map[string]any{"url": url, "transport": "streamable-http"}}
}

func (w *world) syncAll(t *testing.T) {
	t.Helper()
	for _, src := range []string{"files", "openapi", "grpc-reflection"} {
		var op port.Operation
		w.operator().call(t, &op, "syncSource", nil, src)
	}
}

var tenantNames = []string{"ti", "tii", "tiii", "tiv", "tv", "tvi", "tvii", "tviii", "tall", "thidden", "tiso1", "tiso1x", "tiso2", "tiso3", "tpriv", "tgrpc", "trest", "tfiles", "toverlay", "tproj", "tevents", "tcli", "trestart"}

func TestE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	w := &world{t: t, bin: build(t), tenants: map[string]bool{}}
	w.sink, w.mcp, w.rest = newSink(t), newMCPUpstream(t, issueTools()...), newRESTUpstream(t)
	w.drift = newMCPUpstream(t, toolSpec{name: "lookup", description: "Looks a record up", annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}})
	w.grpc = grpctest.Start(t, grpctest.ReflectionV1)

	dir := t.TempDir()
	w.cfg = settings{dir: filepath.Join(dir, "engine"), sinkURL: w.sink.URL, filesDir: filepath.Join(dir, "files"), openAPIDir: filepath.Join(dir, "openapi"), grpcTarget: w.grpc.Target, tenants: tenantNames}
	for _, d := range []string{w.cfg.filesDir, w.cfg.openAPIDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON(t, filepath.Join(w.cfg.filesDir, "issues.json"), remoteListing("acme.dev/issues", "1.0.0", w.mcp.url()))
	writeJSON(t, filepath.Join(w.cfg.filesDir, "drift-1.json"), remoteListing("acme.dev/drift", "1.0.0", w.drift.url()))
	if err := os.WriteFile(filepath.Join(w.cfg.openAPIDir, "petstore.yaml"), []byte(petstoreSpec(w.rest.URL)), 0o600); err != nil {
		t.Fatal(err)
	}
	if pg, err := storetest.StartPostgres(); err != nil {
		t.Logf("running on the memory store: %v", err)
	} else {
		t.Cleanup(pg.Stop)
		if w.cfg.dsn, err = pg.NewDatabase(); err != nil {
			t.Fatal(err)
		}
		w.hasPG = true
	}
	w.eng = startEngine(t, w.bin, w.cfg)
	t.Cleanup(func() { w.eng.stop(t) })
	w.syncAll(t)

	for _, c := range scenarios {
		t.Run(c.name, func(t *testing.T) { c.run(t, w) })
	}
}

var scenarios = []scenario{
	{"owner_i_only_F_for_agent_X", ownerOne},
	{"owner_ii_all_enabled_ask_each_time", ownerAskEachTime},
	{"owner_iii_read_only_for_agent_Y", ownerReadOnly},
	{"owner_iv_ask_once_per_session", ownerAskOnce},
	{"owner_v_workspace_ceiling_cannot_be_widened", ownerCeiling},
	{"owner_vi_description_change_quarantine_accept_confirm", ownerDrift},
	{"owner_vii_new_function_in_update_is_quarantined", ownerNewFunction},
	{"owner_viii_unclassified_function", ownerUnclassified},
	{"owner_enable_all", ownerEnableAll},
	{"tenancy_listing_hidden_for_a_tenant", tenancyHidden},
	{"tenancy_two_tenants_same_names_isolated", tenancyIsolated},
	{"tenancy_private_listing", tenancyPrivate},
	{"idea_files_listing_import_lands_unreviewed", ideaFilesImport},
	{"idea_grpc_kind_call_through_reflection_import", ideaGRPC},
	{"idea_openapi_import_lands_unreviewed_and_calls_rest", ideaOpenAPI},
	{"idea_overlay_accept_and_restore_change_the_served_description", ideaOverlay},
	{"idea_projection_trims_and_result_filter_masks_cpf", ideaProjectionAndFilter},
	{"vsh_tools_list_search_call_write_the_output_file", cliTools},
	{"events_webhook_receives_signed_cloudevents", eventsSigned},
	{"restart_state_and_pending_approval_survive", restartSurvives},
}

type scenario struct {
	name string
	run  func(t *testing.T, w *world)
}

type kit struct {
	w      *world
	tenant string
	conn   port.ConnectorID
	hashes map[string]port.Hash
}

func (w *world) newKit(t *testing.T, tenant string) *kit {
	t.Helper()
	w.operator().call(t, nil, "createTenant", nil, tenant)
	return &kit{w: w, tenant: tenant, hashes: map[string]port.Hash{}}
}

func (k *kit) as(t *testing.T, person string) caller {
	return caller{bin: k.w.bin, url: k.w.eng.HTTP, token: k.w.eng.person(t, k.tenant, person)}
}

func (k *kit) admin(t *testing.T) caller { return k.as(t, "ana") }

func (k *kit) catalogRule(t *testing.T, listing string, ceiling port.Ceiling) {
	t.Helper()
	for _, r := range []port.CatalogRule{
		{SubjectKind: port.SubjectTenant, SubjectID: k.tenant, TargetKind: port.TargetListing, Target: listing, Show: true, Install: true, Ceiling: ceiling},
		{SubjectKind: port.SubjectWorkspace, SubjectID: workspace, TargetKind: port.TargetListing, Target: listing, Show: true, Install: true, Ceiling: port.CeilingFull},
	} {
		k.admin(t).call(t, nil, "putCatalogRule", r)
	}
}

func (k *kit) install(t *testing.T, listing, alias string) port.ConnectorID {
	t.Helper()
	var view service.ConnectorView
	k.admin(t).call(t, &view, "installConnector", map[string]any{"workspace": workspace, "alias": alias, "listing": map[string]any{"name": listing}})
	k.conn = view.Connector.ID
	return k.conn
}

func (k *kit) functions(t *testing.T) map[string]port.Function {
	t.Helper()
	var fns items[port.Function]
	k.admin(t).call(t, &fns, "listFunctions", nil, string(k.conn))
	out := map[string]port.Function{}
	for _, f := range fns.Items {
		out[f.Name] = f
		k.hashes[f.Name] = f.DefinitionHash
	}
	return out
}

func (k *kit) acceptAll(t *testing.T) {
	t.Helper()
	seen := map[string]port.Hash{}
	for name, f := range k.functions(t) {
		if f.State == port.FunctionQuarantined {
			seen[name] = f.DefinitionHash
		}
	}
	k.admin(t).call(t, nil, "acceptFunctions", map[string]any{"seen": seen}, string(k.conn))
}

func (k *kit) classify(t *testing.T, name string, class port.Class) {
	t.Helper()
	k.admin(t).call(t, nil, "classifyFunction", map[string]any{"class": class}, string(k.conn), name)
}

func (k *kit) issuesConnector(t *testing.T, ceiling port.Ceiling) {
	t.Helper()
	k.catalogRule(t, "/acme.dev/issues", ceiling)
	k.install(t, "acme.dev/issues", "s")
	k.acceptAll(t)
	k.classify(t, "search", port.ClassRead)
	k.classify(t, "customer", port.ClassRead)
	k.classify(t, "create_issue", port.ClassWrite)
	k.admin(t).call(t, nil, "setConnectorLevel", map[string]any{"level": port.LevelCustom}, string(k.conn))
}

func (k *kit) grant(t *testing.T, scope port.ScopeKind, scopeID, selector string, mode port.Mode) port.Grant {
	t.Helper()
	g := port.Grant{ScopeKind: scope, ScopeID: scopeID, Connector: k.conn, Selector: selector, Mode: mode}
	if strings.HasPrefix(selector, port.SelectorFunction) {
		g.DefinitionHash = k.hashes[strings.TrimPrefix(selector, port.SelectorFunction)]
	}
	if scope == port.ScopeAgent || scope == port.ScopeUser {
		g.ExpiresAt = time.Now().Add(24 * time.Hour)
	}
	var res service.GrantResult
	k.admin(t).call(t, &res, "createGrant", g)
	return res.Grant
}

func (k *kit) bind(t *testing.T, agent string) {
	t.Helper()
	k.admin(t).call(t, nil, "putBinding", map[string]any{"credential_policy": "none", "ask_requester": true}, string(k.conn), agent)
}

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

type agentSession struct {
	*mcp.ClientSession
	token          string
	kit            *kit
	agent, session string
}

func (k *kit) agent(t *testing.T, agent, session string) agentSession {
	t.Helper()
	tok := k.w.eng.agent(t, agentID{tenant: k.tenant, workspace: workspace, agent: agent, session: session, onBehalfOf: "alice"})
	c := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, nil)
	cs, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: k.w.eng.mcpURL(), HTTPClient: &http.Client{Transport: bearer{tok}}}, nil)
	if err != nil {
		t.Fatalf("agent %s connect: %v", agent, err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return agentSession{ClientSession: cs, token: tok, kit: k, agent: agent, session: session}
}

func (a agentSession) fresh(t *testing.T) agentSession { return a.kit.agent(t, a.agent, a.session) }

func (a agentSession) names(t *testing.T) []string {
	t.Helper()
	res, err := a.fresh(t).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, tool := range res.Tools {
		out = append(out, tool.Name)
	}
	return out
}

func (a agentSession) tools(t *testing.T) map[string]*mcp.Tool {
	t.Helper()
	res, err := a.fresh(t).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*mcp.Tool{}
	for _, tool := range res.Tools {
		out[tool.Name] = tool
	}
	return out
}

func (a agentSession) call(name string, args any) (*mcp.CallToolResult, error) {
	return a.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
}

func (a agentSession) callFields(name string, args any, fields ...string) (*mcp.CallToolResult, error) {
	return a.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args, Meta: mcp.Meta{"connector-engine/fields": fields}})
}

func structured(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(res.StructuredContent)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("structured content %s: %v", raw, err)
	}
	return m
}

func (k *kit) decide(t *testing.T, id string, decision port.Decision) {
	t.Helper()
	k.as(t, "alice").call(t, nil, "decideApproval", map[string]any{"decision": string(decision)}, id)
}
