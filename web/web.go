// Package web is the management panel: the JSON API (package api) paired with
// a browser UI of native ES modules (ui/, no build step) described by
// ui/manifest.json.
//
// A Panel follows the panel module contract of zigbee-go: Handler serves the
// API with relative routes and Assets holds the UI. Standalone serves both.
package web

import (
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"io/fs"
	"net"
	"net/http"
	"path"
	"strings"
)

//go:embed ui
var embedded embed.FS

// UI is the panel's browser assets; manifest.json describes them.
var UI = func() fs.FS {
	sub, err := fs.Sub(embedded, "ui")
	if err != nil {
		panic(err) // the embed directive guarantees the directory exists
	}
	return sub
}()

// Panel is the management panel for one API handler.
type Panel struct{ api http.Handler }

// New returns the panel for an API handler with relative routes.
func New(api http.Handler) *Panel { return &Panel{api: api} }

// Handler serves the API with relative routes.
func (p *Panel) Handler() http.Handler { return p.api }

// Assets returns the browser UI; manifest.json is at its root.
func (p *Panel) Assets() fs.FS { return UI }

// csp only allows the panel's own scripts, styles, images and API calls.
const csp = "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; " +
	"connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

// Standalone serves the panel at /, the API at /api/ and an unauthenticated
// /healthz. A non-empty token is required everywhere else: scripts send
// "Authorization: Bearer <token>", browsers use HTTP basic authentication with
// the token as the password (any user name).
//
// Without a token only requests addressed to a loopback host are served. The
// API controls real devices, and this stops a web page from reaching it through
// DNS rebinding, which origin checks cannot see.
func (p *Panel) Standalone(token string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", p.Handler()))
	mux.Handle("/", static(p.Assets()))
	protected := http.NewCrossOriginProtection().Handler(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok"}`))
			return
		}
		if token == "" && !IsLoopback(r.Host) {
			http.Error(w, "forbidden: set an API token to serve non-loopback hosts", http.StatusForbidden)
			return
		}
		if token != "" && !authorized(r, token) {
			w.Header().Set("WWW-Authenticate", `Basic realm="miservice", charset="UTF-8"`)
			http.Error(w, "unauthorized: use the API token as the password", http.StatusUnauthorized)
			return
		}
		protected.ServeHTTP(w, r)
	})
}

// IsLoopback reports whether a host, with or without a port, is this machine.
func IsLoopback(address string) bool {
	host := address
	if h, _, err := net.SplitHostPort(address); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func authorized(r *http.Request, token string) bool {
	if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return subtle.ConstantTimeCompare([]byte(bearer), []byte(token)) == 1
	}
	_, password, ok := r.BasicAuth()
	return ok && subtle.ConstantTimeCompare([]byte(password), []byte(token)) == 1
}

// static serves an embedded file tree with content-hash ETags, and index.html
// at "/" with the Content-Security-Policy.
func static(files fs.FS) http.Handler {
	etags := map[string]string{}
	fs.WalkDir(files, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if data, err := fs.ReadFile(files, p); err == nil {
				sum := sha256.Sum256(data)
				etags[p] = `"` + hex.EncodeToString(sum[:8]) + `"`
			}
		}
		return nil
	})
	server := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		etag, ok := etags[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		h := w.Header()
		h.Set("ETag", etag)
		h.Set("Cache-Control", "no-cache")
		h.Set("X-Content-Type-Options", "nosniff")
		if name == "index.html" {
			h.Set("Content-Security-Policy", csp)
			http.ServeFileFS(w, r, files, name)
			return
		}
		server.ServeHTTP(w, r)
	})
}
