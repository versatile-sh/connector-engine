package lexical_test

import (
	"context"
	"slices"
	"testing"

	"github.com/agents-control/connector-engine/index/lexical"
	"github.com/agents-control/connector-engine/port"
)

var docs = []port.ToolDoc{
	{Name: "crm__create_contact", Title: "Create contact", Description: "Adds a person to the address book"},
	{Name: "crm__delete_contact", Title: "Delete contact", Description: "Removes a person from the address book"},
	{Name: "mail__send_message", Title: "Send message", Description: "Sends an email to a recipient"},
	{Name: "mail__listInbox", Description: "Lists the newest messages"},
	{Name: "files__read", Description: "Reads a file and its contact sheet"},
}

func names(ds []port.ToolDoc) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Name
	}
	return out
}

func all() []string { return names(docs) }

func setup(t *testing.T) *lexical.Index {
	x := lexical.New()
	if err := x.Put(context.Background(), "t1", docs); err != nil {
		t.Fatal(err)
	}
	return x
}

func TestRanksNameAboveTitleAboveDescription(t *testing.T) {
	got, err := setup(t).Search(context.Background(), "t1", "contact", all(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"crm__create_contact", "crm__delete_contact", "files__read"}; !slices.Equal(names(got), want) {
		t.Fatalf("got %v, want %v", names(got), want)
	}
}

func TestMatchesPrefixesCamelCaseAndLimits(t *testing.T) {
	x := setup(t)
	if got, _ := x.Search(context.Background(), "t1", "inbox", all(), 5); len(got) != 1 || got[0].Name != "mail__listInbox" {
		t.Fatalf("camel case: %v", names(got))
	}
	if got, _ := x.Search(context.Background(), "t1", "mess", all(), 5); !slices.Contains(names(got), "mail__send_message") {
		t.Fatalf("prefix: %v", names(got))
	}
	if got, _ := x.Search(context.Background(), "t1", "contact person", all(), 1); len(got) != 1 || got[0].Name != "crm__create_contact" {
		t.Fatalf("limit: %v", names(got))
	}
	if got, _ := x.Search(context.Background(), "t1", "zebra", all(), 5); len(got) != 0 {
		t.Fatalf("no match: %v", names(got))
	}
}

func TestAllowedRestrictsAndEmptyAllowsNothing(t *testing.T) {
	x := setup(t)
	got, _ := x.Search(context.Background(), "t1", "contact", []string{"files__read", "unknown__tool"}, 10)
	if !slices.Equal(names(got), []string{"files__read"}) {
		t.Fatalf("got %v", names(got))
	}
	if got, _ := x.Search(context.Background(), "t1", "contact", nil, 10); len(got) != 0 {
		t.Fatalf("nothing allowed: %v", names(got))
	}
}

func TestTenantsAreSeparateAndPutReplaces(t *testing.T) {
	x := setup(t)
	if got, _ := x.Search(context.Background(), "t2", "contact", all(), 10); len(got) != 0 {
		t.Fatalf("another tenant sees %v", names(got))
	}
	x.Put(context.Background(), "t1", []port.ToolDoc{{Name: "files__read", Description: "Now about weather"}})
	if got, _ := x.Search(context.Background(), "t1", "contact", all(), 10); slices.Contains(names(got), "files__read") {
		t.Fatalf("a replaced document still matches the old text: %v", names(got))
	}
	if got, _ := x.Search(context.Background(), "t1", "weather", all(), 10); len(got) != 1 {
		t.Fatalf("replacement: %v", names(got))
	}
}
