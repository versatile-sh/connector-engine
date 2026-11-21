package peer

import (
	"net"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// UID is the user id of the process on the other end of c, or -1 where the OS reports none.
func UID(c net.Conn) (int, error) {
	raw, err := c.(syscall.Conn).SyscallConn()
	if err != nil {
		return -1, err
	}
	var cred *unix.Ucred
	var serr error
	if err := raw.Control(func(fd uintptr) { cred, serr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) }); err != nil {
		return -1, err
	}
	if serr != nil {
		return -1, serr
	}
	return int(cred.Uid), nil
}

// OwnerUID is the user id that owns the file, or -1 where the OS reports none.
func OwnerUID(info os.FileInfo) int { return int(info.Sys().(*syscall.Stat_t).Uid) }
