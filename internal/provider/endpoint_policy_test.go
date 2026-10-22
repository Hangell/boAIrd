package provider

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"testing"
	"time"
)

func TestEndpointPolicyAllowsVendorDefaultsAndExplicitHosts(t *testing.T) {
	policy, err := NewEndpointPolicy([]string{"models.example"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name       string
		baseURL    string
		defaultURL string
		wantError  bool
	}{
		{name: "vendor default", baseURL: "https://api.openai.com/v1", defaultURL: "https://api.openai.com/v1"},
		{name: "explicit host", baseURL: "https://models.example/v1"},
		{name: "unapproved host", baseURL: "https://other.example/v1", wantError: true},
		{name: "loopback HTTP", baseURL: "http://127.0.0.1:8080/v1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := policy.validateBaseURL(test.baseURL, test.defaultURL)
			if (err != nil) != test.wantError {
				t.Fatalf("validateBaseURL(%q) error = %v, wantError %t", test.baseURL, err, test.wantError)
			}
		})
	}
}

func TestEndpointPolicyRejectsPrivateAndMetadataAddresses(t *testing.T) {
	policy, err := NewEndpointPolicy([]string{"provider.example"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"10.0.0.1", "169.254.169.254", "100.64.0.1", "168.63.129.16", "192.0.2.1", "fd00::1", "2001:2::1", "2001:db8::1", "::ffff:192.168.1.2"} {
		address := netip.MustParseAddr(value)
		if policy.addressAllowed(address, "provider.example") {
			t.Errorf("address %s was allowed without an explicit IP range", address)
		}
	}
	if !policy.addressAllowed(netip.MustParseAddr("127.0.0.1"), "localhost") {
		t.Fatal("explicit loopback development host was denied")
	}
}

func TestEndpointPolicyRequiresAnApprovedResolvedIPAtDialTime(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	lookup := func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}
	policy, err := NewEndpointPolicy([]string{"provider.example"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	policy.lookupIP = lookup
	if _, err := policy.validateBaseURL("https://provider.example", ""); err != nil {
		t.Fatalf("approved host rejected before DNS lookup: %v", err)
	}
	if _, err := policy.dialContext(context.Background(), "tcp", net.JoinHostPort("provider.example", "443")); err == nil {
		t.Fatal("DNS answer rebinding to loopback was allowed")
	}

	policy, err = NewEndpointPolicy([]string{"provider.example"}, []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatal(err)
	}
	policy.lookupIP = lookup
	accepted := make(chan net.Conn, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- connection
		}
	}()
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	connection, err := policy.dialContext(context.Background(), "tcp", net.JoinHostPort("provider.example", port))
	if err != nil {
		t.Fatalf("dialContext() with approved IP range error = %v", err)
	}
	connection.Close()
	select {
	case acceptedConnection := <-accepted:
		acceptedConnection.Close()
	case <-time.After(time.Second):
		t.Fatal("validated DNS target was not used for the connection")
	}
}

func TestEndpointPolicyAllowsExplicitPrivateCIDR(t *testing.T) {
	policy, err := NewEndpointPolicy([]string{"internal.provider.example"}, []string{"10.20.0.0/16"})
	if err != nil {
		t.Fatal(err)
	}
	if !policy.addressAllowed(netip.MustParseAddr("10.20.4.9"), "internal.provider.example") {
		t.Fatal("explicitly allowed private IP range was denied")
	}
	if policy.addressAllowed(netip.MustParseAddr("10.21.4.9"), "internal.provider.example") {
		t.Fatal("private IP outside the configured range was allowed")
	}
}
