package console

import (
	"context"

	"github.com/versatile-sh/connector-engine/core/listing"
	"github.com/versatile-sh/connector-engine/port"
)

// RepairClasses raises, once per console state, every admin class below the class a function's review or server declares, and records each raise in the audit.
func (c *Console) RepairClasses(ctx context.Context) error {
	if c.State.ClassesRepaired() {
		return nil
	}
	conns, err := c.Engine.liveConnectors(ctx)
	if err != nil {
		return err
	}
	for _, conn := range conns {
		if err := c.repairConnector(ctx, conn); err != nil {
			return err
		}
	}
	return c.State.MarkClassesRepaired()
}

func (c *Console) repairConnector(ctx context.Context, conn port.Connector) error {
	unlock, err := c.lockConnector(ctx, string(conn.ID))
	if err != nil {
		return err
	}
	defer unlock()
	m, err := c.Engine.manifest(ctx, conn)
	if err != nil {
		return err
	}
	fns, err := c.Engine.functions(ctx, string(conn.ID))
	if err != nil {
		return err
	}
	for _, f := range fns {
		base, source := listing.Base(f, m)
		if f.ClassSource != port.ClassSourceAdmin || source != port.ClassSourceReviewed && source != port.ClassSourceAnnotationRaise || !f.Class.Raises(base) {
			continue
		}
		if _, err := c.Engine.setClass(ctx, string(conn.ID), f.Name, base); err != nil {
			return err
		}
		if err := c.State.RecordClassRaised(actorOf(ctx), conn.Alias, f.Name, string(f.Class), string(base)); err != nil {
			return err
		}
	}
	return nil
}
