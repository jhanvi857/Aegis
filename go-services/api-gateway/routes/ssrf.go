package routes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var blockedCIDRs = []*net.IPNet{
	mustParseCIDR("127.0.0.0/8"),    // IPv4 loopback
	mustParseCIDR("10.0.0.0/8"),     // RFC1918 Private class A
	mustParseCIDR("172.16.0.0/12"),  // RFC1918 Private class B
	mustParseCIDR("192.168.0.0/16"), // RFC1918 Private class C
	mustParseCIDR("169.254.0.0/16"), // IPv4 Link-local / AWS / GCP / Azure metadata (169.254.169.254)
	mustParseCIDR("0.0.0.0/8"),      // Current network
	mustParseCIDR("::1/128"),        // IPv6 loopback
	mustParseCIDR("fc00::/7"),       // IPv6 unique local
	mustParseCIDR("fe80::/10"),      // IPv6 link-local
}

func mustParseCIDR(s string) *net.IPNet {
	_, block, err := net.ParseCIDR(s)
	if err != nil {
		panic("invalid CIDR: " + s)
	}
	return block
}

// IsInternalNetworkAllowed returns true if admin explicitly enabled internal metrics/SSRF bypass
func IsInternalNetworkAllowed() bool {
	return strings.EqualFold(os.Getenv("AEGIS_ALLOW_INTERNAL_METRICS"), "true")
}

// ValidateSafeURL checks that the target URL does not resolve to private, loopback, or cloud metadata IP ranges
func ValidateSafeURL(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return errors.New("empty URL")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme '%s', only http and https are allowed", u.Scheme)
	}

	hostname := u.Hostname()
	if hostname == "" {
		return errors.New("missing hostname in URL")
	}

	// Admin explicit bypass for internal testing/development
	if IsInternalNetworkAllowed() {
		return nil
	}

	// Check explicit blocked hostnames
	lowerHost := strings.ToLower(hostname)
	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".localhost") ||
		strings.HasSuffix(lowerHost, ".local") || strings.HasSuffix(lowerHost, ".internal") ||
		lowerHost == "metadata.google.internal" || lowerHost == "169.254.169.254" {
		return fmt.Errorf("SSRF blocked: target host '%s' is in a restricted domain", hostname)
	}

	// Resolve hostname to IP addresses
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// If hostname could not be resolved, treat direct IP parse attempt
		parsedIP := net.ParseIP(hostname)
		if parsedIP != nil {
			ips = []net.IP{parsedIP}
		} else {
			return fmt.Errorf("DNS lookup failed for '%s': %w", hostname, err)
		}
	}

	for _, ip := range ips {
		for _, block := range blockedCIDRs {
			if block.Contains(ip) {
				return fmt.Errorf("SSRF blocked: '%s' resolves to prohibited IP address %s in range %s", hostname, ip.String(), block.String())
			}
		}
	}

	return nil
}

// SafeHTTPClient returns an HTTP client configured with SSRF redirect validation and 5-second timeouts
func SafeHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("stopped after 5 redirects")
			}
			return ValidateSafeURL(req.URL.String())
		},
	}
}

// SafeGet executes an HTTP GET request with SSRF protection, timeout, and a 1MB response size limit
func SafeGet(ctx context.Context, targetURL, authToken, authUser, authPass string) ([]byte, int, error) {
	if err := ValidateSafeURL(targetURL); err != nil {
		return nil, 0, err
	}

	client := SafeHTTPClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, 0, err
	}

	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	} else if authUser != "" || authPass != "" {
		req.SetBasicAuth(authUser, authPass)
	}
	req.Header.Set("User-Agent", "Aegis-Telemetry-Validator/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	// Response size cap: max 1MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, resp.StatusCode, nil
}
