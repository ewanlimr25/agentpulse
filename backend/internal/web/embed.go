// Package web serves the embedded Next.js static export bundled into the
// indie-mode binary. Team mode runs the Next.js app standalone behind its own
// reverse proxy and doesn't use this package.
//
// The static bundle lives under dist/ and is populated by `make indie-bundle`
// (which runs `AGENTPULSE_INDIE_BUILD=1 npm run build` in web/ then copies
// web/out/* into backend/internal/web/dist/). The embed directive picks up
// whatever's there at compile time. When only the .gitkeep is present, the
// handler returns a clear placeholder page so the binary still starts.
package web

import (
	"embed"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler that serves the embedded static bundle with
// SPA-style fallback to /index.html for unknown paths under apiPrefix's siblings.
//
// Routing:
//   - GET /                       → dist/index.html
//   - GET /<asset>                → dist/<asset>
//   - GET /some/route             → dist/some/route.html (Next.js export style)
//                                   or dist/some/route/index.html
//                                   or fallback to dist/index.html (SPA)
//
// apiPrefix is the path prefix reserved for the API (e.g. "/api/" + "/v1/" +
// "/auth/" + "/_health"). Requests under any of those prefixes are NOT served
// by this handler — call ShouldServe to decide.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// embed.FS guarantees this exists at compile time.
		panic("web: embed sub: " + err.Error())
	}
	return &spaHandler{root: sub}
}

// HasBundle reports whether a real Next.js export is embedded (i.e., dist/
// contains an index.html). When false, the indie binary still starts but the
// UI handler returns a placeholder explaining how to populate the bundle.
func HasBundle() bool {
	_, err := fs.Stat(distFS, "dist/index.html")
	return err == nil
}

// APIPrefixes is the set of URL path prefixes that belong to the API + OTLP +
// health endpoints. Requests matching any of these prefixes must skip the
// embedded UI handler.
var APIPrefixes = []string{
	"/api/",
	"/v1/", // OTLP + ingest
	"/auth/",
	"/_health",
	"/health",
	"/metrics",
}

// IsAPIPath reports whether p starts with one of APIPrefixes.
func IsAPIPath(p string) bool {
	for _, pre := range APIPrefixes {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	return false
}

type spaHandler struct {
	root fs.FS
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	urlPath := strings.TrimPrefix(r.URL.Path, "/")
	if urlPath == "" {
		urlPath = "index.html"
	}

	// Try the literal asset first.
	if served := h.tryServe(w, r, urlPath); served {
		return
	}
	// Next.js static-export sometimes emits foo/page.tsx → foo.html (no slash)
	// and sometimes foo/index.html (with `trailingSlash: true`). Try both.
	if !strings.HasSuffix(urlPath, ".html") && !strings.Contains(path.Base(urlPath), ".") {
		if served := h.tryServe(w, r, urlPath+".html"); served {
			return
		}
		if served := h.tryServe(w, r, path.Join(urlPath, "index.html")); served {
			return
		}
	}

	// SPA fallback — serve index.html so client-side routing handles it.
	// If the bundle is empty (no index.html), surface the placeholder.
	if HasBundle() {
		_ = h.tryServe(w, r, "index.html")
		return
	}
	servePlaceholder(w)
}

func (h *spaHandler) tryServe(w http.ResponseWriter, r *http.Request, name string) bool {
	f, err := h.root.Open(name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}
	if stat.IsDir() {
		return false
	}

	rs, ok := f.(io.ReadSeeker)
	if !ok {
		// embed.FS files are ReadSeekers; this should be unreachable.
		w.Header().Set("Content-Type", contentTypeFor(name))
		_, _ = io.Copy(w, f)
		return true
	}
	http.ServeContent(w, r, name, stat.ModTime(), rs)
	return true
}

func contentTypeFor(name string) string {
	switch {
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		return "application/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".json"):
		return "application/json; charset=utf-8"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}

const placeholderHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>AgentPulse — UI bundle missing</title>
<style>
body { font-family: system-ui, sans-serif; max-width: 640px; margin: 4rem auto; padding: 0 1rem; color: #222; line-height: 1.5; }
code { background: #f4f4f4; padding: 0.15rem 0.35rem; border-radius: 3px; }
.note { background: #fff7e6; border-left: 4px solid #ffa940; padding: 1rem 1.25rem; border-radius: 4px; }
</style>
</head>
<body>
<h1>AgentPulse is running</h1>
<p>The API is up at <code>/api/v1/*</code> and the OTLP receiver is listening on the configured port. However, the embedded web UI bundle has not been built into this binary.</p>
<div class="note">
<strong>To bundle the UI:</strong>
<pre><code>make indie-bundle
go build -tags=duckdb ./cmd/server</code></pre>
This runs the Next.js static export and embeds the resulting <code>web/out/</code> into <code>backend/internal/web/dist/</code> at compile time.
</div>
<p>You can still use AgentPulse via the CLI or by pointing your existing Next.js dev server at this API.</p>
</body>
</html>`

func servePlaceholder(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, placeholderHTML)
}
