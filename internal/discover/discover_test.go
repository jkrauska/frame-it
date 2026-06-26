package discover

import (
	"net"
	"testing"

	"github.com/koron/go-ssdp"
)

func TestIsSamsungSSDP(t *testing.T) {
	tests := []struct {
		name string
		svc  ssdp.Service
		want bool
	}{
		{
			name: "server header",
			svc:  ssdp.Service{Server: "Samsung-Linux/3.14 UPnP/1.0"},
			want: true,
		},
		{
			name: "usn header",
			svc:  ssdp.Service{USN: "uuid:068e7780-006e-1000-bc6f-90f1aaca27dc::urn:samsung.com:device:RemoteControlReceiver:1"},
			want: true,
		},
		{
			name: "sonos",
			svc:  ssdp.Service{Server: "Linux UPnP/1.0 Sonos/95.1-78010 (ZPS19)"},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSamsungSSDP(tc.svc); got != tc.want {
				t.Fatalf("isSamsungSSDP() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHostFromLocation(t *testing.T) {
	tests := []struct {
		loc  string
		want string
	}{
		{"http://192.168.1.50:8001/api/v2/", "192.168.1.50"},
		{"https://192.168.1.50/description.xml", "192.168.1.50"},
		{"not-a-url", ""},
	}

	for _, tc := range tests {
		if got := hostFromLocation(tc.loc); got != tc.want {
			t.Fatalf("hostFromLocation(%q) = %q, want %q", tc.loc, got, tc.want)
		}
	}
}

func TestLanSearchAddrsReturnsLANBinding(t *testing.T) {
	addrs, err := lanSearchAddrs()
	if err != nil {
		t.Fatalf("lanSearchAddrs() error: %v", err)
	}
	if len(addrs) == 0 {
		t.Fatal("lanSearchAddrs() returned no addresses")
	}
	for _, addr := range addrs {
		if addr == "" {
			continue
		}
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			t.Fatalf("invalid addr %q: %v", addr, err)
		}
		if host == "" || port != "0" {
			t.Fatalf("unexpected addr %q", addr)
		}
	}
}

func TestSSDPBindingReceivesLANResponses(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping SSDP integration test in short mode")
	}

	ifaces, err := lanInterfaces()
	if err != nil {
		t.Fatalf("lanInterfaces() error: %v", err)
	}
	if len(ifaces) == 0 {
		t.Skip("no LAN interfaces available")
	}

	prev := ssdp.Interfaces
	ssdp.Interfaces = ifaces
	defer func() { ssdp.Interfaces = prev }()

	services, err := ssdp.Search(ssdp.All, 3, "")
	if err != nil {
		t.Fatalf("ssdp.Search() error: %v", err)
	}
	if len(services) == 0 {
		t.Fatal("ssdp.Search() returned no services; LAN interface binding may be broken")
	}
}
