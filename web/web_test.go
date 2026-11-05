package web

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"regexp"
	"strings"
	"testing"
)

func TestManifestFollowsContract(t *testing.T) {
	data, err := fs.ReadFile(UI, "manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Contract int
		ID       string
		Title    string
		Entry    string
		Styles   []string
		Nav      []struct{ Path string }
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.Contract != 1 || m.ID != "miservice" || m.Title == "" {
		t.Fatalf("manifest = %+v", m)
	}
	for _, name := range append([]string{m.Entry, "index.html"}, m.Styles...) {
		if _, err := fs.Stat(UI, name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	entry, _ := fs.ReadFile(UI, m.Entry)
	if !regexp.MustCompile(`export (function|const) mount\b`).Match(entry) {
		t.Error("entry does not export mount")
	}
	for _, n := range m.Nav {
		if !strings.Contains(string(entry), `path: '`+n.Path+`'`) {
			t.Errorf("no route for %s", n.Path)
		}
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
	New(api).Standalone(token).ServeHTTP(w, req)
	return w
}

func TestStandaloneRoutes(t *testing.T) {
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
	New(api).Standalone("").ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d", w.Code)
	}
}
