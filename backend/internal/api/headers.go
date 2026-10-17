package api

import (
	"net/http"
	"path"
	"strings"
)

// contentSecurityPolicy lets the app load only its own scripts, styles, fonts and images and
// talk only to its own origin (including the simulation WebSocket). Inline styles are allowed
// because React sets style attributes; inline scripts are not.
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"font-src 'self'; img-src 'self' data:; connect-src 'self' ws: wss:; manifest-src 'self'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// secureHeaders adds browser hardening headers to every response.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

// cacheControl says how long a static response may be cached: build output under /assets/ has a
// content hash in its name and never changes, everything else (index.html, the theme script) must
// be revalidated so a new deploy is picked up immediately.
func cacheControl(urlPath string) string {
	if strings.HasPrefix(path.Clean("/"+urlPath), "/assets/") {
		return "public, max-age=31536000, immutable"
	}
	return "no-cache"
}
