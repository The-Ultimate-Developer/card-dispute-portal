package httpx

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP returns the caller's address when the API sits behind a known number
// of proxies. Each proxy appends the address it received the request from to
// X-Forwarded-For, so the real client is `hops` entries from the right. Anything
// further left is client-controlled and can be forged, so it is never trusted.
func ClientIP(r *http.Request, trustedHops int) string {
	if trustedHops > 0 {
		parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		if idx := len(parts) - trustedHops; idx >= 0 && r.Header.Get("X-Forwarded-For") != "" {
			if ip := net.ParseIP(strings.TrimSpace(parts[idx])); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
