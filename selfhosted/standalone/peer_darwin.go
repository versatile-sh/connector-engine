package standalone

import (
	"net"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func peerUID(c net.Conn) (int, error) {
	raw, err := c.(syscall.Conn).SyscallConn()
	if err != nil {
		return -1, err
	}
	var cred *unix.Xucred
	var serr error
	if err := raw.Control(func(fd uintptr) { cred, serr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED) }); err != nil {
		return -1, err
	}
	if serr != nil {
		return -1, serr
	}
	return int(cred.Uid), nil
}

func ownerUID(info os.FileInfo) int { return int(info.Sys().(*syscall.Stat_t).Uid) }
