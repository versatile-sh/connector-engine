// Package egress refuses outbound dials to loopback, link-local, private and metadata addresses unless the host is allowed.
package egress

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"slices"

	"google.golang.org/grpc"

	"github.com/agents-control/connector-engine/port"
)

var carrierGrade = netip.MustParsePrefix("100.64.0.0/10")

// Guard decides which hosts an adapter may dial; the zero value refuses every non-public address.
type Guard struct {
	AllowPrivate bool
	Hosts        []string
}

// Forbidden reports whether ip is loopback, link-local, private, unspecified, multicast, shared or a metadata address.
func Forbidden(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || carrierGrade.Contains(ip)
}

func listed(hosts []string, addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	return slices.Contains(hosts, host) || slices.Contains(hosts, addr)
}

// DialContext dials addr after resolving it and checking every address; a non-empty extra narrows the dial to the hosts it lists.
func (g Guard) DialContext(extra ...string) func(ctx context.Context, network, addr string) (net.Conn, error) {
	var d net.Dialer
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if len(extra) > 0 && !listed(extra, addr) {
			return nil, port.Errorf(port.CodePermissionDenied, "egress to %s is refused: it is not in egress.allow_hosts", addr)
		}
		if g.AllowPrivate || listed(g.Hosts, addr) {
			return d.DialContext(ctx, network, addr)
		}
		host, portNum, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, port.Errorf(port.CodeInvalidArgument, "egress address %q: %v", addr, err)
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, port.Errorf(port.CodeUnavailable, "egress resolve %s: %v", host, err)
		}
		for _, ip := range ips {
			if Forbidden(ip) {
				return nil, port.Errorf(port.CodePermissionDenied, "egress to %s is refused: it resolves to the non-public address %s", host, ip)
			}
		}
		if len(ips) == 0 {
			return nil, port.Errorf(port.CodeUnavailable, "egress resolve %s: no address", host)
		}
		return d.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), portNum))
	}
}

// Transport returns a clone of the default HTTP transport that dials through the guard.
func (g Guard) Transport(extra ...string) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = g.DialContext(extra...)
	return t
}

// Client returns an HTTP client that dials through the guard.
func (g Guard) Client(extra ...string) *http.Client {
	return &http.Client{Transport: g.Transport(extra...)}
}

// GRPC returns the gRPC dial option that dials through the guard.
func (g Guard) GRPC(extra ...string) grpc.DialOption {
	dial := g.DialContext(extra...)
	return grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) { return dial(ctx, "tcp", addr) })
}
