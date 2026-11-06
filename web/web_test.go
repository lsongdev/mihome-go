package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"regexp"
	"strings"
	"testing"
)

// index.html has to load the app and stylesheets that exist.
func TestIndexLoadsApp(t *testing.T) {
	index, err := fs.ReadFile(UI, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	refs := regexp.MustCompile(`(?:src|href)="([^":]+)"`).FindAllSubmatch(index, -1)
	app := false
	for _, m := range refs {
		name := string(m[1])
		app = app || name == "app.js"
		if _, err := fs.Stat(UI, name); err != nil {
			t.Errorf("index.html loads %s: %v", name, err)
		}
	}
	if !app {
		t.Error("index.html does not load app.js")
	}
}

// Every relative ES module import in the UI must resolve to an embedded file.
func TestModuleImportsResolve(t *testing.T) {
	imports := regexp.MustCompile(`(?:from|import)\s*\(?\s*'(\.{1,2}/[^']+)'`)
	fs.WalkDir(UI, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(name, "node_modules/") || !strings.HasSuffix(name, ".js") {
			return err
		}
		source, _ := fs.ReadFile(UI, name)
		for _, m := range imports.FindAllStringSubmatch(string(source), -1) {
			if _, err := fs.Stat(UI, path.Join(path.Dir(name), m[1])); err != nil {
				t.Errorf("%s imports missing %s", name, m[1])
			}
		}
		return nil
	})
}

func serve(token, host, target string, prepare func(*http.Request)) *httptest.ResponseRecorder {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("api:" + r.URL.Path)) })
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Host = host
	if prepare != nil {
		prepare(req)
	}
	w := httptest.NewRecorder()
	Handler(api, token).ServeHTTP(w, req)
	return w
}

func TestHandlerRoutes(t *testing.T) {
	if w := serve("", "127.0.0.1:8080", "/api/homes", nil); w.Code != 200 || w.Body.String() != "api:/homes" {
		t.Fatalf("api: %d %s", w.Code, w.Body)
	}
	w := serve("", "localhost:8080", "/", nil)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatalf("index: %d %v", w.Code, w.Header())
	}
	if w := serve("", "localhost:8080", "/missing.js", nil); w.Code != 404 {
		t.Fatalf("missing: %d", w.Code)
	}
}

func TestWithoutTokenOnlyLoopbackHostsAreServed(t *testing.T) {
	for host, code := range map[string]int{
		"127.0.0.1:8080": 200, "localhost:8080": 200, "[::1]:8080": 200, "localhost": 200,
		"evil.example:8080": 403, "192.168.1.2:8080": 403, "localhost.evil.example": 403,
	} {
		if w := serve("", host, "/api/homes", nil); w.Code != code {
			t.Errorf("%s: got %d, want %d", host, w.Code, code)
		}
	}
	if w := serve("", "evil.example", "/healthz", nil); w.Code != 200 {
		t.Errorf("healthz: %d", w.Code)
	}
}

func TestTokenIsRequired(t *testing.T) {
	host := "192.168.1.2:8080"
	w := serve("secret", host, "/api/homes", nil)
	if w.Code != 401 || w.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("anonymous: %d", w.Code)
	}
	if w := serve("secret", host, "/api/homes", func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong") }); w.Code != 401 {
		t.Fatalf("wrong token: %d", w.Code)
	}
	if w := serve("secret", host, "/api/homes", func(r *http.Request) { r.Header.Set("Authorization", "Bearer secret") }); w.Code != 200 {
		t.Fatalf("bearer: %d", w.Code)
	}
	if w := serve("secret", host, "/api/homes", func(r *http.Request) { r.SetBasicAuth("me", "secret") }); w.Code != 200 {
		t.Fatalf("basic: %d", w.Code)
	}
	if w := serve("secret", host, "/healthz", nil); w.Code != 200 {
		t.Fatalf("healthz: %d", w.Code)
	}
}

func TestCrossOriginWritesAreRejected(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("request reached the API") })
	req := httptest.NewRequest(http.MethodPost, "/api/speakers/s1/tts", strings.NewReader(`{}`))
	req.Host = "localhost:8080"
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	Handler(api, "").ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d", w.Code)
	}
}
