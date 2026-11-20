package console_test

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	secretslocal "github.com/versatile-sh/connector-engine/secrets/local"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
	"github.com/versatile-sh/connector-engine/selfhosted/console/sealed"
)

func TestAnAgentRequestToAnEngineThatIsDownGets503WithAJSONRPCError(t *testing.T) {
	gone, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	engine := "http://" + gone.Addr().String()
	gone.Close()
	open := &sealed.Box{}
	store, err := secretslocal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	open.Unlock(store)
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"agent":"a","session":"s","iat":1}`))
	for name, box := range map[string]*sealed.Box{"locked": {}, "not answering": open} {
		t.Run(name, func(t *testing.T) {
			state, err := console.OpenState(t.TempDir() + "/state.json")
			if err != nil {
				t.Fatal(err)
			}
			c := &console.Console{Engine: &console.Engine{PublicURL: engine}, State: state, Sessions: console.NewSessions(state), Secrets: box, Guard: console.Guard{Hosts: []string{"gw.test"}}}
			req := httptest.NewRequest(http.MethodPost, "http://gw.test/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
			req.Header.Set("Authorization", "Bearer x."+claims+".y")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c.Handler().ServeHTTP(rec, req)
			var body struct {
				JSONRPC string
				Error   struct{ Message string }
			}
			if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") == "" || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.JSONRPC != "2.0" || !strings.Contains(body.Error.Message, "make unlock") {
				t.Fatalf("%d Retry-After %q body %s", rec.Code, rec.Header().Get("Retry-After"), rec.Body)
			}
			if locked := strings.Contains(body.Error.Message, "is locked"); locked != (name == "locked") {
				t.Fatalf("message %q for a %s gateway", body.Error.Message, name)
			}
		})
	}
}
