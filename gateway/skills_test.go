package gateway_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/gateway"
	"github.com/agents-control/connector-engine/port"
	rtskill "github.com/agents-control/connector-engine/runtime/skill"
	"github.com/agents-control/connector-engine/service"
	st "github.com/agents-control/connector-engine/service/servicetest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	skillMD = "---\nname: pdf\ndescription: Fills PDF forms.\n---\nRun scripts/fill.sh with the form."
	script  = "#!/bin/sh\necho filled\n"
)

func skillFile(p, content string) port.SkillFile {
	sum := sha256.Sum256([]byte(content))
	return port.SkillFile{Path: p, Bytes: int64(len(content)), SHA256: hex.EncodeToString(sum[:]), Content: []byte(content)}
}

func skillManifest() port.Manifest {
	commit := strings.Repeat("c", 40)
	return port.Manifest{Name: "skills.example/pdf", Version: commit, Title: "PDF", Description: "Fills PDF forms.", Kind: port.KindSkill, Publisher: port.Publisher{Name: "example"},
		Skill: &port.Skill{Name: "pdf", Source: port.SkillSource{Repository: "https://github.com/example/skills", Path: "pdf", Commit: commit},
			Licence: port.SkillLicence{SPDX: "MIT", Redistributable: true}, Redistribution: port.RedistributionBundled, HasScripts: true, Requires: []string{"com.notion/mcp"},
			Files: []port.SkillFile{skillFile("SKILL.md", skillMD), skillFile("scripts/fill.sh", script)}}}
}

func (w *world) installSkill(mode port.Mode, agents ...string) port.ConnectorID {
	w.t.Helper()
	m := skillManifest()
	sref := port.ListingRef{Name: m.Name}
	op := w.Operator()
	st.Ok(w.Svc.CreateListing(w.Ctx, op, service.CreateListingInput{Manifest: m}))
	for _, r := range []port.CatalogRule{
		{SubjectKind: port.SubjectTenant, SubjectID: string(tenant), TargetKind: port.TargetListing, Target: sref.String(), Show: true, Install: true, Ceiling: port.CeilingFull},
		{SubjectKind: port.SubjectWorkspace, SubjectID: string(workspace), TargetKind: port.TargetListing, Target: sref.String(), Show: true, Install: true, Ceiling: port.CeilingFull},
	} {
		st.Ok(w.Svc.PutCatalogRule(w.Ctx, w.admin, r))
	}
	conn := st.Ok(w.Svc.InstallConnector(w.Ctx, w.admin, service.InstallInput{Workspace: workspace, Alias: "pdf", Listing: sref})).Connector
	fns := st.Ok(w.Svc.ListFunctionsOf(w.Ctx, w.admin, conn.ID))
	if len(fns) != 1 || fns[0].State != port.FunctionQuarantined {
		w.t.Fatalf("a skill lands as one quarantined function: %+v", fns)
	}
	st.Ok(w.Svc.AcceptFunctions(w.Ctx, w.admin, conn.ID, map[string]port.Hash{rtskill.Function: fns[0].DefinitionHash}))
	st.Ok(w.Svc.SetConnectorLevel(w.Ctx, w.admin, conn.ID, port.LevelCustom))
	st.Ok(w.Svc.ClassifyFunction(w.Ctx, w.admin, conn.ID, rtskill.Function, port.ClassRead))
	hash := fns[0].DefinitionHash
	st.Ok(w.Svc.CreateGrant(w.Ctx, w.admin, port.Grant{ScopeKind: port.ScopeWorkspace, ScopeID: string(workspace), Connector: conn.ID, Selector: port.SelectorFunction + rtskill.Function, Mode: port.ModeAllow, DefinitionHash: hash}))
	for _, a := range agents {
		st.Ok(w.Svc.PutBinding(w.Ctx, w.admin, port.Binding{Connector: conn.ID, Agent: port.AgentID(a), AskRequester: true}))
		st.Ok(w.Svc.CreateGrant(w.Ctx, w.admin, port.Grant{ScopeKind: port.ScopeAgent, ScopeID: a, Agent: port.AgentID(a), Connector: conn.ID, Selector: port.SelectorFunction + rtskill.Function,
			Mode: mode, DefinitionHash: hash, ExpiresAt: st.Epoch.Add(30 * 24 * time.Hour)}))
	}
	return conn.ID
}

func skillsOn(c *gateway.Config) { c.Skills = true }

func texts(r *mcp.CallToolResult) []string {
	var out []string
	for _, c := range r.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			out = append(out, t.Text)
		}
	}
	return out
}

func TestSkillsThroughTheAgentEndpoint(t *testing.T) {
	for _, era := range []string{"2025-11-25", "2026-07-28"} {
		t.Run("in protocol "+era, func(t *testing.T) {
			w := newWorld(t, options{cfg: skillsOn})
			w.bind("a1")
			w.row(port.ScopeAgent, "a1", "*", port.ModeAllow)
			w.installSkill(port.ModeAllow, "a1")
			cs, _ := w.agent("a1", "s1", era)

			t.Run("lists the two meta-tools and never the skill as a tool", func(t *testing.T) {
				got := names(t, cs)
				if !slices.Contains(got, "skill_list") || !slices.Contains(got, "skill_load") || slices.ContainsFunc(got, func(n string) bool { return strings.HasPrefix(n, "pdf") }) {
					t.Fatalf("%v", got)
				}
			})
			t.Run("refuses the skill's function through tools/call and call", func(t *testing.T) {
				if res, err := callTool(cs, "pdf__skill", map[string]any{}); err == nil && !res.IsError {
					t.Fatal("a skill ran as a tool")
				}
				if res, err := callTool(cs, "call", map[string]any{"name": "pdf__skill"}); err == nil && !res.IsError {
					t.Fatal("a skill ran through call")
				}
			})
			t.Run("skill_list notes the unbound required connector", func(t *testing.T) {
				res, err := callTool(cs, "skill_list", map[string]any{})
				raw, _ := json.Marshal(res.StructuredContent)
				if err != nil || !strings.Contains(string(raw), `"name":"pdf"`) || !strings.Contains(string(raw), "needs connector com.notion/mcp, not connected") {
					t.Fatalf("%v %s", err, raw)
				}
			})
			t.Run("skill_load serves SKILL.md and a script as text", func(t *testing.T) {
				res, err := callTool(cs, "skill_load", map[string]any{"skill": "pdf"})
				if err != nil || res.IsError || len(texts(res)) != 2 || texts(res)[1] != skillMD {
					t.Fatalf("%v %v", err, texts(res))
				}
				res, err = callTool(cs, "skill_load", map[string]any{"skill": "pdf", "path": "scripts/fill.sh"})
				if err != nil || res.IsError || texts(res)[1] != script {
					t.Fatalf("%v %v", err, texts(res))
				}
			})
			t.Run("the instructions index names the skill", func(t *testing.T) {
				init := cs.InitializeResult()
				if init == nil || !strings.Contains(init.Instructions, "\n- pdf: Fills PDF forms.") {
					t.Fatalf("%+v", init)
				}
			})
		})
	}
}

func TestAnAgentWithoutTheSkillDoesNotSeeIt(t *testing.T) {
	w := newWorld(t, options{cfg: skillsOn})
	w.bind("a1")
	w.row(port.ScopeAgent, "a1", "*", port.ModeAllow)
	w.installSkill(port.ModeAllow)
	cs, _ := w.agent("a1", "s1", "2025-11-25")
	res, err := callTool(cs, "skill_load", map[string]any{"skill": "pdf"})
	if err == nil && !res.IsError {
		t.Fatalf("an unbound agent loaded the skill: %v", texts(res))
	}
	if strings.Contains(cs.InitializeResult().Instructions, "pdf") {
		t.Fatal("the index names a skill the agent does not have")
	}
}

func TestASkillOnAskHoldsTheLoad(t *testing.T) {
	w := newWorld(t, options{cfg: skillsOn})
	w.installSkill(port.ModeAskOnce, "a1")
	cs, _ := w.agent("a1", "s1", "2025-11-25")
	res, err := callTool(cs, "skill_load", map[string]any{"skill": "pdf"})
	raw, _ := json.Marshal(res.StructuredContent)
	if err != nil || !strings.Contains(string(raw), "pending_approval") || !slices.Contains(names(t, cs), "approval_status") {
		t.Fatalf("%v %s", err, raw)
	}
	id := port.ApprovalID(structuredOf(t, res)["approval_id"].(string))
	st.Ok(w.Svc.DecideApproval(w.Ctx, w.alice, id, port.DecisionApproveForSession, "", "test"))
	for range 2 {
		if _, err := w.Worker.Sweep(w.Ctx); err != nil {
			t.Fatal(err)
		}
	}
	status, err := callTool(cs, "approval_status", map[string]any{"approval_id": string(id)})
	if err != nil || status.IsError || len(texts(status)) != 2 || texts(status)[1] != skillMD {
		t.Fatalf("after approval: %v %v", err, texts(status))
	}
	again, err := callTool(cs, "skill_load", map[string]any{"skill": "pdf", "path": "scripts/fill.sh"})
	if err != nil || again.IsError || len(texts(again)) != 2 || texts(again)[1] != script {
		t.Fatalf("ask once lets the rest of the session load: %v %v", err, texts(again))
	}
}

func TestTheMetaToolsSurviveCollapse(t *testing.T) {
	w := newWorld(t, options{cfg: func(c *gateway.Config) { c.Skills, c.ThresholdFunctions = true, 1 }})
	w.bind("a1")
	w.row(port.ScopeAgent, "a1", "*", port.ModeAllow)
	w.installSkill(port.ModeAllow, "a1")
	cs, _ := w.agent("a1", "s1", "2025-11-25")
	res, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, tool := range res.Tools {
		got = append(got, tool.Name)
		if strings.HasPrefix(tool.Name, "skill_") && tool.Meta["anthropic/alwaysLoad"] != true {
			t.Errorf("%s lacks alwaysLoad", tool.Name)
		}
	}
	if len(got) < 4 || !slices.Equal(got[:2], []string{"find", "call"}) || !slices.Equal(got[len(got)-2:], []string{"skill_list", "skill_load"}) {
		t.Fatalf("%v", got)
	}
}

func TestWithSkillsOffTheEndpointIsUnchanged(t *testing.T) {
	w := newWorld(t, options{})
	w.bind("a1")
	w.row(port.ScopeAgent, "a1", "*", port.ModeAllow)
	w.installSkill(port.ModeAllow, "a1")
	for _, era := range []string{"2025-11-25", "2026-07-28"} {
		cs, _ := w.agent("a1", "s-"+era, era)
		got := names(t, cs)
		if slices.ContainsFunc(got, func(n string) bool { return strings.HasPrefix(n, "skill_") || strings.HasPrefix(n, "pdf") }) {
			t.Fatalf("%s: %v", era, got)
		}
		if res, err := callTool(cs, "skill_load", map[string]any{"skill": "pdf"}); err == nil && !res.IsError {
			t.Fatalf("%s: skill_load answered with skills off", era)
		}
		if i := cs.InitializeResult(); i == nil || strings.Contains(i.Instructions, "skill") {
			t.Fatalf("%s: %+v", era, i)
		}
	}
}

func TestTheSkillIndexStaysUnder4096Characters(t *testing.T) {
	var skills []service.ServedSkill
	for i := range 40 {
		skills = append(skills, service.ServedSkill{Name: fmt.Sprintf("skill-%d", i), Description: strings.Repeat("words ", 40), Mode: port.ModeAllow})
	}
	got := gateway.SkillIndex("Base.", skills)
	if len(got) > 4096 || !strings.HasSuffix(got, "call skill_list.") || !strings.HasPrefix(got, "Base.") {
		t.Fatalf("%d %q", len(got), got[len(got)-60:])
	}
	if gateway.SkillIndex("Base.", nil) != "Base." {
		t.Fatal("an agent without skills gets an index")
	}
}
