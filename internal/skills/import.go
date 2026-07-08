package skills

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

const (
	importTimeout  = 10 * time.Second
	importMaxBytes = 1 << 20 // 1 MiB, matching the API body-limit middleware
)

// metadataIP is the cloud instance-metadata address — a classic SSRF target.
var metadataIP = net.ParseIP("169.254.169.254")

// cgnatNet is the RFC 6598 carrier-grade-NAT range (100.64.0.0/10), used inside
// many cloud/k8s networks (some clouds host metadata here) — not covered by
// net.IP.IsPrivate.
var cgnatNet = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// ErrImportBlocked is returned when a fetch URL fails the scheme/SSRF policy.
var ErrImportBlocked = errors.New("skill import: URL blocked by fetch policy")

// newImportClient builds the bounded HTTP client used for skill imports. Two
// layers guard against SSRF:
//   - CheckRedirect re-validates each hop's URL (cheap early reject).
//   - a dial-time Control hook re-checks the ACTUAL connected IP, which closes
//     the DNS-rebinding TOCTOU: validateFetchURL resolves+checks names up front,
//     but the transport re-resolves at dial time, so a rebinding DNS answer
//     could otherwise reach an internal/metadata host. Control runs on the real
//     address the dialer connects to, so validation and connection see the same
//     IP.
func newImportClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: importTimeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return ErrImportBlocked
			}
			return blockInternalIP(ip)
		},
	}
	return &http.Client{
		Timeout:   importTimeout,
		Transport: &http.Transport{DialContext: dialer.DialContext},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("skill import: too many redirects")
			}
			return validateFetchURL(req.URL)
		},
	}
}

// blockInternalIP is the scheme-independent dial-time guard: it rejects any
// internal/metadata/unspecified address regardless of the URL scheme (the
// scheme-aware rules — e.g. http only to loopback — are enforced pre-dial by
// validateFetchURL). Loopback and public addresses are allowed here.
func blockInternalIP(ip net.IP) error {
	switch {
	case ip.Equal(metadataIP):
		return fmt.Errorf("%w: metadata address", ErrImportBlocked)
	case ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast():
		return fmt.Errorf("%w: link-local address", ErrImportBlocked)
	case ip.IsUnspecified():
		return fmt.Errorf("%w: unspecified address", ErrImportBlocked)
	case ip.IsLoopback():
		return nil
	case ip.IsPrivate() || cgnatNet.Contains(ip):
		return fmt.Errorf("%w: private address", ErrImportBlocked)
	}
	return nil
}

// validateFetchURL enforces the scheme allowlist and SSRF policy: https is
// allowed to public hosts; http is allowed only to loopback (dev); link-local,
// the metadata IP, and private ranges are always blocked. A hostname is
// resolved and every resolved IP must pass.
func validateFetchURL(u *url.URL) error {
	switch u.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("%w: scheme %q not allowed", ErrImportBlocked, u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: missing host", ErrImportBlocked)
	}

	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		resolved, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("%w: cannot resolve %q: %v", ErrImportBlocked, host, err)
		}
		ips = resolved
	}
	for _, ip := range ips {
		if err := checkIP(ip, u.Scheme); err != nil {
			return err
		}
	}
	return nil
}

func checkIP(ip net.IP, scheme string) error {
	switch {
	case ip.Equal(metadataIP):
		return fmt.Errorf("%w: metadata address", ErrImportBlocked)
	case ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast():
		return fmt.Errorf("%w: link-local address", ErrImportBlocked)
	case ip.IsUnspecified():
		return fmt.Errorf("%w: unspecified address", ErrImportBlocked)
	case ip.IsLoopback():
		return nil // dev carve-out: loopback is allowed for http and https
	case scheme == "http":
		return fmt.Errorf("%w: http is allowed only for loopback", ErrImportBlocked)
	case ip.IsPrivate() || cgnatNet.Contains(ip):
		return fmt.Errorf("%w: private address", ErrImportBlocked)
	}
	return nil
}

// fetchSkill downloads and parses a skill from a URL under the bounded, SSRF-safe
// policy. It never registers a partial skill: any error (blocked URL, transport
// failure, oversize body, bad skill document) returns a zero Skill and an error.
func fetchSkill(client *http.Client, rawURL string) (Skill, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return Skill{}, fmt.Errorf("skill import: bad URL: %w", err)
	}
	if err := validateFetchURL(u); err != nil {
		return Skill{}, err
	}
	if client == nil {
		client = newImportClient()
	}
	// Fetching a user-supplied URL is the feature (skill import). The SSRF risk is
	// mitigated in depth and does not rely on this call site trusting the input:
	// validateFetchURL (above) enforces the scheme allowlist and rejects
	// metadata/link-local/private/CGNAT/unspecified hosts pre-dial; the client's
	// dial-time Control hook (blockInternalIP) re-checks the ACTUAL connected IP,
	// closing the DNS-rebinding TOCTOU; CheckRedirect re-validates every hop. A
	// CodeQL "uncontrolled data in network request" alert here is expected and
	// accepted given these layered guards.
	resp, err := client.Get(u.String())
	if err != nil {
		return Skill{}, fmt.Errorf("skill import: fetch failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Skill{}, fmt.Errorf("skill import: unexpected status %d", resp.StatusCode)
	}

	// Read at most importMaxBytes+1 so we can detect an oversize body.
	data, err := io.ReadAll(io.LimitReader(resp.Body, importMaxBytes+1))
	if err != nil {
		return Skill{}, fmt.Errorf("skill import: read failed: %w", err)
	}
	if len(data) > importMaxBytes {
		return Skill{}, fmt.Errorf("skill import: body exceeds %d bytes", importMaxBytes)
	}
	return ParseSkill(data, false)
}
