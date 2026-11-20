package console

import (
	"context"
	"testing"
)

func TestTheDefaultSkillFetchReachesPublicHTTPSOnly(t *testing.T) {
	get := (&Console{}).get()
	for _, raw := range []string{"http://catalog.example/pdf.yaml", "https://127.0.0.1/pdf.yaml", "https://10.0.0.1/pdf.yaml", "https://user:pw@catalog.example/pdf.yaml"} {
		if _, err := get(context.Background(), raw, 1024); err == nil {
			t.Errorf("%s was fetched", raw)
		}
	}
}
