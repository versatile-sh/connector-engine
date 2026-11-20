package console

import (
	"fmt"

	"github.com/agents-control/connector-engine/surface"
)

// DefaultChannel builds the built-in approval channels.
func DefaultChannel(kind string, settings map[string]string) (surface.Channel, error) {
	return nil, fmt.Errorf("channel kind %q is not built in", kind)
}
