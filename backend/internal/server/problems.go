package server

import (
	"net/http"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/httpx"
)

// problemFallback converts the router's built-in plain-text 404 and 405 replies
// into our RFC 7807 problem+json format, while keeping the router's behaviour
// (including the Allow header on 405).
//
// Why not a catch-all "/" route? Because a catch-all matches every path, so the
// router never gets to say "path exists, wrong method" and 405 becomes 404.
func problemFallback(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&problemWriter{ResponseWriter: w, r: r}, r)
	})
}

type problemWriter struct {
	http.ResponseWriter
	r        *http.Request
	replaced bool
}

func (p *problemWriter) WriteHeader(code int) {
	// Our own handlers always set a JSON content type first; the router's
	// default errors are text/plain. Only rewrite those.
	if (code == http.StatusNotFound || code == http.StatusMethodNotAllowed) &&
		p.Header().Get("Content-Type") == "text/plain; charset=utf-8" {
		p.replaced = true
		detail := "No route matches this path."
		if code == http.StatusMethodNotAllowed {
			detail = "This method is not allowed for this path."
		}
		p.Header().Del("Content-Length")
		httpx.WriteProblem(p.ResponseWriter, p.r, code, detail)
		return
	}
	p.ResponseWriter.WriteHeader(code)
}

func (p *problemWriter) Write(b []byte) (int, error) {
	if p.replaced {
		return len(b), nil // swallow the router's plain-text body
	}
	return p.ResponseWriter.Write(b)
}
