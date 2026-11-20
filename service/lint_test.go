package service_test

import (
	"testing"

	"github.com/versatile-sh/connector-engine/port"
	st "github.com/versatile-sh/connector-engine/service/servicetest"
)

func TestLintCaptureNeedsTheCreatorOrReviewerRole(t *testing.T) {
	w := newWorld(t, opts{})
	m := st.Manifest(listing, "2.0.0")
	capture := w.Svc.Capture

	plain, err := w.Svc.LintListing(w.Ctx, w.alice, m, nil)
	want(t, err, nil, "a person lints a manifest")
	want(t, len(plain.Definitions), 0, "no capture, no definitions")

	_, err = w.Svc.LintListing(w.Ctx, w.alice, m, capture)
	want(t, port.IsCode(err, port.CodePermissionDenied), true, "a person without a role may not capture")
	_, err = w.Svc.LintListing(w.Ctx, w.agent("a1", "s1"), m, capture)
	want(t, port.IsCode(err, port.CodeAgentPrincipalRefused), true, "an agent may not capture")

	got, err := w.Svc.LintListing(w.Ctx, w.admin, m, capture)
	want(t, err, nil, "a tenant admin captures")
	want(t, len(got.Definitions) > 0, true, "the definitions came back")
	got, err = w.Svc.LintListing(w.Ctx, w.Operator(), m, capture)
	want(t, err, nil, "the operator captures")
	want(t, len(got.Definitions) > 0, true, "the operator sees the definitions")
}
