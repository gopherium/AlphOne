// SPDX-License-Identifier: Elastic-2.0

package webhook

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// ErrAddressRefused reports an outbound connection to an address the guard does not allow.
var ErrAddressRefused = errors.New("webhook: address refused")

// internal lists the ranges no outbound connection reaches unless an operator allows them.
var internal = prefixes(
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
	"192.0.0.0/24", "192.168.0.0/16", "198.18.0.0/15", "224.0.0.0/4", "240.0.0.0/4",
	"::/128", "::1/128", "64:ff9b:1::/48", "fc00::/7", "fe80::/10", "ff00::/8",
)

// never lists the link-local and cloud metadata ranges no operator can allow.
var never = prefixes("169.254.0.0/16", "fe80::/10", "fd00:ec2::254/128")

// translated is the well-known NAT64 prefix, whose addresses carry the IPv4 address they reach in their last bytes.
var translated = netip.MustParsePrefix("64:ff9b::/96")

// prefixes parses the ranges a table lists.
func prefixes(ranges ...string) []netip.Prefix {
	parsed := make([]netip.Prefix, 0, len(ranges))
	for _, held := range ranges {
		parsed = append(parsed, netip.MustParsePrefix(held))
	}
	return parsed
}

// AllowList names the internal destinations an operator lets outbound requests reach.
type AllowList struct {
	// Ranges are the internal address ranges a dialled address may fall in.
	Ranges []netip.Prefix
}

// AddressGuard decides which addresses outbound requests may reach.
type AddressGuard struct {
	// Allowed names the internal destinations an operator opened.
	Allowed AllowList
	// Resolver looks host names up, nil using the system resolver.
	Resolver *net.Resolver
}

// Client returns an HTTP client bounded by timeout that dials only allowed addresses and ignores proxy settings.
func (g AddressGuard) Client(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = g.dial
	return &http.Client{Timeout: timeout, Transport: transport}
}

// dial connects to address once every address it resolves to passes the guard.
func (g AddressGuard) dial(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := net.Dialer{Resolver: g.Resolver, ControlContext: g.control}
	return dialer.DialContext(ctx, network, address)
}

// control refuses a connection about to open to an address the guard does not allow.
func (g AddressGuard) control(_ context.Context, network, address string, _ syscall.RawConn) error {
	return g.check(network, address)
}

// check refuses an address never allowed and an internal address outside the allowed ranges.
func (g AddressGuard) check(network, address string) error {
	held, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("%w: %s %s is not an address", ErrAddressRefused, network, address)
	}
	ip := reached(held.Addr().Unmap().WithZone(""))
	switch {
	case within(ip, never):
		return fmt.Errorf("%w: %s %s is never reachable", ErrAddressRefused, network, address)
	case within(ip, g.Allowed.Ranges):
		return nil
	case within(ip, internal):
		return fmt.Errorf("%w: %s %s is an internal address", ErrAddressRefused, network, address)
	}
	return nil
}

// reached returns the address a connection to ip ends at, the IPv4 address a translated one carries.
func reached(ip netip.Addr) netip.Addr {
	if !translated.Contains(ip) {
		return ip
	}
	held := ip.As16()
	return netip.AddrFrom4([4]byte(held[12:]))
}

// within reports whether any of ranges holds ip.
func within(ip netip.Addr, ranges []netip.Prefix) bool {
	for _, held := range ranges {
		if held.Contains(ip) {
			return true
		}
	}
	return false
}
