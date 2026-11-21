// Package peer restricts a unix listener to connections from the user that runs the process (or root).
package peer

import "net"

// Only wraps lis so that Accept returns only connections whose peer is the running user or root and closes the others; an OS without peer credentials relies on the 0700 directory.
func Only(lis net.Listener, uid int) net.Listener { return listener{Listener: lis, uid: uid} }

type listener struct {
	net.Listener
	uid int
}

func (l listener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if uid, err := UID(c); err == nil && (uid < 0 || uid == 0 || uid == l.uid) {
			return c, nil
		}
		c.Close()
	}
}
