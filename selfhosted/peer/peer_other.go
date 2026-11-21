//go:build !linux && !darwin

package peer

import (
	"net"
	"os"
)

// UID is the user id of the process on the other end of c, or -1 where the OS reports none.
func UID(net.Conn) (int, error) { return -1, nil }

// OwnerUID is the user id that owns the file, or -1 where the OS reports none.
func OwnerUID(os.FileInfo) int { return -1 }
