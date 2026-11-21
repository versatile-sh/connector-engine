package console

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const listingURL = "https://catalog.test/skills/catalog/pdf.yaml"

type listingFeed struct {
	body  []byte
	err   error
	calls int
}

func (f *listingFeed) get(context.Context, string, int64) ([]byte, error) {
	f.calls++
	return f.body, f.err
}

func fixtureListing(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "source", "skillcatalog", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func iconConsole(t *testing.T, feed *listingFeed) *Console {
	t.Helper()
	state, err := OpenState(filepath.Join(t.TempDir(), "state.json"), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.PutOrigin("s1", Origin{Listing: listingURL}); err != nil {
		t.Fatal(err)
	}
	return &Console{State: state, Fetch: feed.get}
}

func TestASkillWithoutAnIconGetsTheOneItsListingCarriesAndKeepsIt(t *testing.T) {
	feed := &listingFeed{body: fixtureListing(t, "pdf.yaml")}
	c := iconConsole(t, feed)
	list := []SkillSummary{{ID: "s1"}}
	c.backfillIcons(context.Background(), list)
	if list[0].Icon != "https://versatile.sh/avatars/example.png" {
		t.Fatalf("icon %q", list[0].Icon)
	}
	if o, _ := c.State.OriginOf("s1"); o.Icon != list[0].Icon || o.Listing != listingURL {
		t.Fatalf("the state keeps %+v", o)
	}
}

func TestASkillThatHasAnIconIsNotRead(t *testing.T) {
	feed := &listingFeed{body: fixtureListing(t, "pdf.yaml")}
	c := iconConsole(t, feed)
	list := []SkillSummary{{ID: "s1", Icon: "https://versatile.sh/avatars/own.png"}}
	c.backfillIcons(context.Background(), list)
	if feed.calls != 0 || list[0].Icon != "https://versatile.sh/avatars/own.png" {
		t.Fatalf("%d reads, icon %q", feed.calls, list[0].Icon)
	}
}

func TestASkillWithoutARecordedListingIsLeftAlone(t *testing.T) {
	feed := &listingFeed{body: fixtureListing(t, "pdf.yaml")}
	c := iconConsole(t, feed)
	list := []SkillSummary{{ID: "unknown"}}
	c.backfillIcons(context.Background(), list)
	if feed.calls != 0 || list[0].Icon != "" {
		t.Fatalf("%d reads, icon %q", feed.calls, list[0].Icon)
	}
}

func TestAListingWithoutAnIconIsReadOnceAnHour(t *testing.T) {
	feed := &listingFeed{body: fixtureListing(t, "pdf-unlicensed.yaml")}
	c := iconConsole(t, feed)
	for range 3 {
		list := []SkillSummary{{ID: "s1"}}
		c.backfillIcons(context.Background(), list)
		if list[0].Icon != "" {
			t.Fatalf("icon %q", list[0].Icon)
		}
	}
	if feed.calls != 1 {
		t.Fatalf("%d reads", feed.calls)
	}
}

func TestAListingThatCannotBeReadNeverFailsTheList(t *testing.T) {
	feed := &listingFeed{err: errors.New("offline")}
	c := iconConsole(t, feed)
	list := []SkillSummary{{ID: "s1", Title: "PDF"}}
	c.backfillIcons(context.Background(), list)
	if list[0].Icon != "" || list[0].Title != "PDF" || feed.calls != 1 {
		t.Fatalf("%+v after %d reads", list[0], feed.calls)
	}
}

func TestAnIconThatIsNotHTTPSIsDropped(t *testing.T) {
	body := append(fixtureListing(t, "pdf-unlicensed.yaml"), "icon: http://versatile.sh/avatars/plain.png\n"...)
	c := iconConsole(t, &listingFeed{body: body})
	list := []SkillSummary{{ID: "s1"}}
	c.backfillIcons(context.Background(), list)
	if list[0].Icon != "" {
		t.Fatalf("icon %q", list[0].Icon)
	}
}
