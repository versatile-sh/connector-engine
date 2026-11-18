package remote

import (
	"encoding/json"
	"testing"
)

func TestAPageSentButNotCapturedIsAMissNotTheCache(t *testing.T) {
	in := &instance{capture: &capture{}, pages: map[string]page{"": {Tools: []json.RawMessage{json.RawMessage(`{"name":"old"}`)}}}}
	if _, ok := in.pageFor(""); !ok {
		t.Fatal("with no request on the wire the SDK cache answered: the cached page is served")
	}
	rec := &recorded{id: json.RawMessage(`1`), contentType: "application/json"}
	rec.write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[]}}`))
	in.capture.list = []*recorded{rec}
	if _, ok := in.pageFor(""); ok {
		t.Fatal("a response that was sent but not captured must not fall back to the cached page")
	}
}
