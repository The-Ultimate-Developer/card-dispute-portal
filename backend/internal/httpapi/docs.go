package httpapi

import (
	"net/http"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/api"
)

const swaggerVersion = "5.17.14"

const docsPage = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Card Dispute Portal API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@` + swaggerVersion + `/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@` + swaggerVersion + `/swagger-ui-bundle.js" crossorigin></script>
  <script src="/docs/init.js"></script>
</body>
</html>`

const docsInit = `window.ui = SwaggerUIBundle({
  url: "/openapi.yaml",
  dom_id: "#swagger-ui",
  withCredentials: true,
  persistAuthorization: true
});`

// The API's default CSP blocks all scripts. The docs page needs Swagger UI from
// a pinned CDN version, so it gets its own narrow policy and nothing broader.
const docsCSP = "default-src 'none'; script-src 'self' https://unpkg.com; " +
	"style-src 'self' 'unsafe-inline' https://unpkg.com; img-src 'self' data: https://unpkg.com; " +
	"connect-src 'self'; frame-ancestors 'none'"

func registerDocs(mux *http.ServeMux) {
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Security-Policy", docsCSP)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(docsPage))
	})
	mux.HandleFunc("GET /docs/init.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write([]byte(docsInit))
	})
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(api.OpenAPI)
	})
}
