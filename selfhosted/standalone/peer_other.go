//go:build !linux && !darwin

package standalone

import (
	"net"
	"os"
)

func peerUID(net.Conn) (int, error) { return -1, nil }

func ownerUID(os.FileInfo) int { return -1 }
