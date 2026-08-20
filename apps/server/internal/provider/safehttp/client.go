package safehttp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var carrierGradeNAT = netip.MustParsePrefix("100.64.0.0/10")

func NewClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            safeDialContext,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           20,
		IdleConnTimeout:        30 * time.Second,
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  15 * time.Second,
		MaxResponseHeaderBytes: 1 << 20,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) > 5 {
				return errors.New("too many redirects")
			}
			return ValidateURL(request.URL)
		},
	}
}

func ParsePublicURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, err
	}
	if err := ValidateURL(parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func ValidateURL(parsed *url.URL) error {
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("only http and https URLs are allowed")
	}
	if parsed.Hostname() == "" || parsed.User != nil {
		return errors.New("URL must have a host and no credentials")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return errors.New("local hosts are not allowed")
	}
	if address, err := netip.ParseAddr(host); err == nil && !isPublicAddress(address) {
		return errors.New("non-public IP addresses are not allowed")
	}
	return nil
}

func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	var addresses []netip.Addr
	if parsed, parseErr := netip.ParseAddr(host); parseErr == nil {
		addresses = []netip.Addr{parsed}
	} else {
		addresses, err = net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
	}
	if len(addresses) == 0 {
		return nil, errors.New("host has no IP address")
	}
	for _, candidate := range addresses {
		if !isPublicAddress(candidate) {
			return nil, fmt.Errorf("host resolves to non-public address %s", candidate)
		}
	}

	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	var lastError error
	for _, candidate := range addresses {
		connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		lastError = dialErr
	}
	return nil, lastError
}

func isPublicAddress(address netip.Addr) bool {
	address = address.Unmap()
	return address.IsValid() && address.IsGlobalUnicast() && !address.IsPrivate() &&
		!address.IsLoopback() && !address.IsLinkLocalUnicast() &&
		!(address.Is4() && carrierGradeNAT.Contains(address))
}
