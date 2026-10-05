// SPDX-License-Identifier: Elastic-2.0

package webhook

import (
	"errors"
	"net/http"
	"net/netip"
	"testing"
	"time"
)

// ranges parses the allowed ranges a test names.
func ranges(t *testing.T, held ...string) []netip.Prefix {
	t.Helper()
	parsed := make([]netip.Prefix, 0, len(held))
	for _, raw := range held {
		parsed = append(parsed, netip.MustParsePrefix(raw))
	}
	return parsed
}

func TestTheGuardRefusesEveryInternalRange(t *testing.T) {
	t.Parallel()

	for name, address := range map[string]string{
		"this network":                 "0.0.0.0:80",
		"this network at its end":      "0.255.255.255:80",
		"a private ten":                "10.0.0.1:80",
		"a private ten for pods":       "10.42.0.10:5432",
		"a private ten at its end":     "10.255.255.255:80",
		"shared address space":         "100.64.0.1:80",
		"shared address space end":     "100.127.255.255:80",
		"loopback":                     "127.0.0.1:8080",
		"loopback elsewhere":           "127.8.9.10:8080",
		"loopback at its end":          "127.255.255.255:8080",
		"cloud metadata":               "169.254.169.254:80",
		"a private seventeen":          "172.16.0.1:80",
		"a container bridge":           "172.17.0.2:5432",
		"a private seventeen end":      "172.31.255.255:80",
		"protocol assignments":         "192.0.0.8:80",
		"protocol assignments end":     "192.0.0.255:80",
		"a private one ninety":         "192.168.1.1:80",
		"a private one ninety end":     "192.168.255.255:80",
		"benchmarking":                 "198.18.0.1:80",
		"benchmarking at its end":      "198.19.255.255:80",
		"multicast":                    "224.0.0.1:80",
		"multicast at its end":         "239.255.255.255:80",
		"reserved for the next":        "240.0.0.1:80",
		"broadcast":                    "255.255.255.255:80",
		"unspecified six":              "[::]:80",
		"loopback six":                 "[::1]:80",
		"local translation":            "[64:ff9b:1::a00:1]:80",
		"local translation at its end": "[64:ff9b:1:ffff:ffff:ffff:ffff:ffff]:80",
		"unique local six":             "[fc00::1]:80",
		"a container network six":      "[fd12:3456::1]:5432",
		"unique local six far":         "[fdff::1]:80",
		"unique local six at its end":  "[fdff:ffff:ffff:ffff:ffff:ffff:ffff:ffff]:80",
		"metadata six":                 "[fd00:ec2::254]:80",
		"link local six":               "[fe80::1]:80",
		"link local six at its end":    "[febf:ffff:ffff:ffff:ffff:ffff:ffff:ffff]:80",
		"translated six":               "[64:ff9b::a00:1]:80",
		"mapped loopback":              "[::ffff:127.0.0.1]:80",
		"mapped private":               "[::ffff:10.0.0.1]:80",
		"zoned link local":             "[fe80::1%eth0]:80",
		"multicast six":                "[ff02::1]:80",
		"multicast six at its end":     "[ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff]:80",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if err := (AddressGuard{}).check("tcp", address); !errors.Is(err, ErrAddressRefused) {
				t.Errorf("check(%q) error = %v, want %v", address, err, ErrAddressRefused)
			}
		})
	}
}

func TestTheGuardPassesPublicAddressesOnAnyPort(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		"192.0.2.10:443", "198.51.100.7:5678", "203.0.113.9:80", "[2001:db8::1]:8443",
		"1.0.0.1:443", "9.255.255.255:443", "11.0.0.1:443", "100.63.255.255:443", "100.128.0.1:443",
		"126.255.255.255:443", "128.0.0.1:443", "169.253.255.255:443", "169.255.0.1:443",
		"172.15.255.255:443", "172.32.0.1:443", "192.0.1.1:443", "192.167.255.255:443", "192.169.0.1:443",
		"198.17.255.255:443", "198.20.0.1:443", "223.255.255.255:443",
	} {
		if err := (AddressGuard{}).check("tcp", address); err != nil {
			t.Errorf("check(%q) error = %v, want a public address passed", address, err)
		}
	}
}

func TestTheGuardPassesAnInternalAddressInsideAnAllowedRange(t *testing.T) {
	t.Parallel()

	guard := AddressGuard{Allowed: AllowList{Ranges: ranges(t, "127.0.0.1/32")}}

	if err := guard.check("tcp4", "127.0.0.1:5678"); err != nil {
		t.Errorf("check() inside the allowed range error = %v, want nil", err)
	}
	if err := guard.check("tcp4", "127.0.0.2:5678"); !errors.Is(err, ErrAddressRefused) {
		t.Errorf("check() beside the allowed range error = %v, want %v", err, ErrAddressRefused)
	}
}

// neverReached holds link-local and metadata addresses across their ranges, refused whatever an operator allows.
var neverReached = []string{
	"169.254.0.1:80", "169.254.169.254:80", "169.254.170.2:80", "169.254.255.255:80",
	"[fe80::1]:80", "[fe80::1%eth0]:80", "[fe80:1::1]:80", "[febf::1]:80", "[fd00:ec2::254]:80",
	"[64:ff9b::a9fe:a9fe]:80", "[64:ff9b::a9fe:aa02]:80",
}

func TestTheGuardNeverPassesLinkLocalOrMetadataAddresses(t *testing.T) {
	t.Parallel()

	guard := AddressGuard{Allowed: AllowList{Ranges: ranges(t,
		"0.0.0.0/0", "::/0", "169.254.0.0/16", "fe80::/10", "fd00::/8", "64:ff9b::/96")}}

	for _, address := range neverReached {
		if err := guard.check("tcp", address); !errors.Is(err, ErrAddressRefused) {
			t.Errorf("check(%q) error = %v, want it refused whatever the allowed ranges", address, err)
		}
	}
	if err := guard.check("tcp", "10.0.0.1:80"); err != nil {
		t.Errorf("check() inside an allowed range error = %v, want the rest of the range passed", err)
	}
}

func TestAHostEntryNeverOpensATranslatedMetadataAddress(t *testing.T) {
	t.Parallel()

	guard := AddressGuard{Allowed: AllowList{Hosts: []string{"n8n:5678"}}}.naming("n8n:5678")

	if err := guard.check("tcp", "[64:ff9b::a9fe:a9fe]:5678"); !errors.Is(err, ErrAddressRefused) {
		t.Errorf("check() error = %v, want the translated metadata address refused behind a host entry", err)
	}
	if err := guard.check("tcp", "[64:ff9b::a00:5]:5678"); err != nil {
		t.Errorf("check() error = %v, want the host entry to open a translated internal address", err)
	}
}

func TestAHostEntryNeverOpensALinkLocalOrMetadataAddress(t *testing.T) {
	t.Parallel()

	guard := AddressGuard{Allowed: AllowList{Hosts: []string{"n8n:5678"}}}.naming("n8n:5678")

	for _, address := range neverReached {
		if err := guard.check("tcp", address); !errors.Is(err, ErrAddressRefused) {
			t.Errorf("check(%q) error = %v, want it refused behind a host entry", address, err)
		}
	}
	if err := guard.check("tcp", "10.0.0.5:5678"); err != nil {
		t.Errorf("check() error = %v, want the host entry to open an internal address", err)
	}
}

func TestTheGuardJudgesATranslatedAddressByTheIPv4AddressItCarries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		allowed []string
		address string
		refused bool
	}{
		{name: "a public address", address: "[64:ff9b::c000:20a]:443"},
		{name: "a private address", address: "[64:ff9b::a00:1]:80", refused: true},
		{name: "a private address inside an allowed range", allowed: []string{"10.0.0.0/8"}, address: "[64:ff9b::a00:1]:80"},
		{name: "a private address behind an allowed translator range", allowed: []string{"64:ff9b::/96"},
			address: "[64:ff9b::a00:1]:80", refused: true},
		{name: "loopback", address: "[64:ff9b::7f00:1]:80", refused: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := AddressGuard{Allowed: AllowList{Ranges: ranges(t, tc.allowed...)}}.check("tcp6", tc.address)

			if refused := errors.Is(err, ErrAddressRefused); refused != tc.refused {
				t.Errorf("check(%q) error = %v, want refused %v", tc.address, err, tc.refused)
			}
		})
	}
}

func TestTheGuardRefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()

	if err := (AddressGuard{}).check("unix", "/run/postgresql/.s.PGSQL.5432"); !errors.Is(err, ErrAddressRefused) {
		t.Errorf("check() error = %v, want an unreadable address refused", err)
	}
}

func TestTheClientIgnoresTheProxySettingsAndKeepsItsTimeout(t *testing.T) {
	t.Parallel()

	client := (AddressGuard{}).Client(7 * time.Second)

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport = %T, want an *http.Transport", client.Transport)
	}
	if transport.Proxy != nil {
		t.Error("transport proxy is set, want every proxy setting ignored so the guard sees the real target")
	}
	if transport == http.DefaultTransport {
		t.Error("client shares http.DefaultTransport, want a pool of its own")
	}
	if client.Timeout != 7*time.Second {
		t.Errorf("client timeout = %v, want 7s", client.Timeout)
	}
}
