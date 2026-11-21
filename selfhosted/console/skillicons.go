package console

import (
	"context"
	"sync"
	"time"

	"github.com/versatile-sh/connector-engine/source/skillcatalog"
)

const (
	iconBackfillWait = 4 * time.Second
	iconRetryAfter   = time.Hour
)

type iconTries struct {
	mu sync.Mutex
	at map[string]time.Time
}

func (t *iconTries) due(id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if last, ok := t.at[id]; ok && time.Since(last) < iconRetryAfter {
		return false
	}
	put(&t.at, id, time.Now())
	return true
}

func (c *Console) listedIcon(ctx context.Context, listing string) string {
	ctx, cancel := context.WithTimeout(ctx, iconBackfillWait)
	defer cancel()
	body, err := c.get()(ctx, listing, maxListingBytes)
	if err != nil {
		return ""
	}
	e, err := skillcatalog.Parse(body)
	if err != nil {
		return ""
	}
	return httpsOnly(e.IconAt(listing))
}

// backfillIcons gives each listed skill without an icon the one its recorded listing carries now and keeps it in the console state; a listing that cannot be read or has no icon is tried again after an hour, and none of it ever fails the list.
func (c *Console) backfillIcons(ctx context.Context, list []SkillSummary) {
	var wg sync.WaitGroup
	for i := range list {
		s := &list[i]
		origin, ok := c.State.OriginOf(s.ID)
		if s.Icon != "" || !ok || origin.Listing == "" || !c.iconTries.due(s.ID) {
			continue
		}
		wg.Go(func() {
			if s.Icon = c.listedIcon(ctx, origin.Listing); s.Icon != "" {
				_ = c.State.SetOriginIcon(s.ID, s.Icon)
			}
		})
	}
	wg.Wait()
}
