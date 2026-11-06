// Package web is the management panel: the JSON API (package api) paired with
// a browser UI of native ES modules (ui/, no build step).
package web

import (
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"io/fs"
	"net"
	"net/http"
	"path"
	"regexp"
	"strings"
)

//go:embed ui
var embedded embed.FS

// UI is the panel's browser assets.
var UI = func() fs.FS {
	sub, err := fs.Sub(embedded, "ui")
	if err != nil {
		panic(err) // the embed directive guarantees the directory exists
	}
	return sub
}()

// csp allows the panel's own scripts, styles, images and API calls, and the
// inline import map of index.html by its hash.
func csp(index []byte) string {
	scripts := "'self'"
	for _, m := range importMap.FindAllSubmatch(index, -1) {
		sum := sha256.Sum256(m[1])
		scripts += " 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	}
	return "script-src " + scripts + "; " + policy
}

// importMap finds inline import maps, which a script-src without a hash blocks.
var importMap = regexp.MustCompile(`(?s)<script type="importmap">(.*?)</script>`)

const policy = "default-src 'self'; img-src 'self' data:; media-src 'self' blob:; style-src 'self' 'unsafe-inline'; " +
	"connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

// Handler serves the panel at /, the API at /api/ and an unauthenticated
// /healthz. A non-empty token is required everywhere else: scripts send
// "Authorization: Bearer <token>", browsers use HTTP basic authentication with
// the token as the password (any user name).
//
// Without a token only requests addressed to a loopback host are served. The
// API controls real devices, and this stops a web page from reaching it through
// DNS rebinding, which origin checks cannot see.
func Handler(api http.Handler, token string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", api))
	mux.Handle("/", static(UI))
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
			w.Header().Set("WWW-Authenticate", `Basic realm="mihome", charset="UTF-8"`)
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
	index, _ := fs.ReadFile(files, "index.html")
	security := csp(index)
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
			h.Set("Content-Security-Policy", security)
			http.ServeFileFS(w, r, files, name)
			return
		}
		server.ServeHTTP(w, r)
	})
}
