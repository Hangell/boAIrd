package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// EndpointPolicy controls which provider hosts and resolved addresses adapters may reach.
type EndpointPolicy struct {
	allowedHosts    map[string]struct{}
	allowedIPRanges []netip.Prefix
	lookupIP        func(context.Context, string) ([]netip.Addr, error)
	dialer          func(context.Context, string, string) (net.Conn, error)
}

// specialPurposePrefixes mirrors the active IPv6 entries in the IANA Special-Purpose
// Address Registry (updated 2025-10-09), plus transition prefixes that can carry
// embedded IPv4 destinations. Review this list when the registry changes.
var specialPurposePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("168.63.129.16/32"), // Azure platform and metadata virtual IP.
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"), // Well-known IPv4/IPv6 translation; may encode non-public IPv4 targets.
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("100:0:0:1::/64"),
	netip.MustParsePrefix("2001::/23"), // IETF assignments, including Teredo.
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"), // 6to4 embeds an IPv4 destination.
	netip.MustParsePrefix("2620:4f:8000::/48"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
}

// NewEndpointPolicy creates a policy from exact hostnames and explicit destination ranges.
func NewEndpointPolicy(allowedHosts, allowedIPRanges []string) (EndpointPolicy, error) {
	policy := EndpointPolicy{allowedHosts: make(map[string]struct{}, len(allowedHosts))}
	for _, host := range allowedHosts {
		normalized, err := normalizePolicyHost(host)
		if err != nil {
			return EndpointPolicy{}, fmt.Errorf("invalid provider endpoint host: %w", err)
		}
		policy.allowedHosts[normalized] = struct{}{}
	}
	for _, rawRange := range allowedIPRanges {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(rawRange))
		if err != nil {
			return EndpointPolicy{}, fmt.Errorf("invalid provider endpoint IP range %q", rawRange)
		}
		policy.allowedIPRanges = append(policy.allowedIPRanges, prefix.Masked())
	}
	return policy, nil
}

func (p EndpointPolicy) validateBaseURL(value, defaultBaseURL string) (*url.URL, error) {
	parsed, err := validateBaseURL(value)
	if err != nil {
		return nil, err
	}
	if address, err := netip.ParseAddr(parsed.Hostname()); err == nil && address.Zone() != "" {
		return nil, errors.New("provider endpoint IP zones are not allowed")
	}
	host := normalizeURLHost(parsed.Hostname())
	if isExplicitLoopbackHost(host) || host == normalizeURLHost(hostnameOf(defaultBaseURL)) {
		return parsed, nil
	}
	if _, allowed := p.allowedHosts[host]; !allowed {
		return nil, errors.New("custom provider host is not in the configured allowlist")
	}
	return parsed, nil
}

func (p EndpointPolicy) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errors.New("provider endpoint address is invalid")
	}
	host = normalizeURLHost(host)
	addresses := make([]netip.Addr, 0, 1)
	if literal, err := netip.ParseAddr(host); err == nil {
		addresses = append(addresses, literal.Unmap())
	} else {
		lookup := p.lookupIP
		if lookup == nil {
			lookup = func(ctx context.Context, host string) ([]netip.Addr, error) {
				return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			}
		}
		addresses, err = lookup(ctx, host)
		if err != nil {
			return nil, errors.New("provider endpoint DNS resolution failed")
		}
	}
	if len(addresses) == 0 {
		return nil, errors.New("provider endpoint resolved to no addresses")
	}
	for _, address := range addresses {
		if !p.addressAllowed(address, host) {
			return nil, errors.New("provider endpoint resolved to a disallowed IP address")
		}
	}
	var lastErr error
	for _, address := range addresses {
		dial := p.dialer
		if dial == nil {
			dial = (&net.Dialer{}).DialContext
		}
		connection, err := dial(ctx, network, net.JoinHostPort(address.String(), port))
		if err == nil {
			return connection, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("provider endpoint connection failed: %w", lastErr)
}

func (p EndpointPolicy) addressAllowed(address netip.Addr, host string) bool {
	address = address.Unmap()
	if isExplicitLoopbackHost(host) {
		return address.IsLoopback()
	}
	if isPrivateOrSpecialAddress(address) {
		for _, prefix := range p.allowedIPRanges {
			if prefix.Contains(address) {
				return true
			}
		}
		return false
	}
	return true
}

func isPrivateOrSpecialAddress(address netip.Addr) bool {
	if !address.IsValid() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsUnspecified() || address.IsMulticast() {
		return true
	}
	for _, prefix := range specialPurposePrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func normalizePolicyHost(host string) (string, error) {
	host = normalizeURLHost(strings.TrimSpace(host))
	if host == "" || strings.ContainsAny(host, "/@?#\t\r\n *") {
		return "", errors.New("host must be an exact hostname or IP address")
	}
	if address, err := netip.ParseAddr(host); err == nil {
		if address.Zone() != "" {
			return "", errors.New("IP zones are not allowed")
		}
		return host, nil
	}
	if len(host) > 253 {
		return "", errors.New("hostname is too long")
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", errors.New("hostname is invalid")
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return "", errors.New("hostname must use ASCII letters, digits, and hyphens")
			}
		}
	}
	return host, nil
}

func hostnameOf(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func normalizeURLHost(host string) string {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if address, err := netip.ParseAddr(host); err == nil {
		return address.Unmap().String()
	}
	return host
}

func isExplicitLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address, err := netip.ParseAddr(host)
	return err == nil && address.IsLoopback()
}
