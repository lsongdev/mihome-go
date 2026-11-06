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

var (
	jsImport  = regexp.MustCompile(`(?:from\s+|import\s*\(\s*|import\s+)'([^']+)'`)
	cssImport = regexp.MustCompile(`@import url\(([^)]+)\)`)
)

// imports reads the import map of index.html: bare specifier prefixes and
// the paths they stand for.
func imports(t *testing.T) map[string]string {
	index, err := fs.ReadFile(UI, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	var m struct{ Imports map[string]string }
	for _, found := range importMap.FindAllSubmatch(index, -1) {
		if err := json.Unmarshal(found[1], &m); err != nil {
			t.Fatalf("import map: %v", err)
		}
	}
	return m.Imports
}

// Every module a page imports, directly or through the import map, has to be
// in the embedded tree. A missing node_modules means the browser
// dependencies were not installed.
func TestImportsResolve(t *testing.T) {
	if _, err := fs.Stat(UI, "node_modules/@lsongdev/scripts/package.json"); err != nil {
		t.Fatalf("browser dependencies are missing, run make deps: %v", err)
	}
	mapped := imports(t)
	resolve := func(from, spec string) (string, bool) {
		if strings.HasPrefix(spec, ".") {
			return path.Join(path.Dir(from), spec), true
		}
		for prefix, target := range mapped {
			if strings.HasPrefix(spec, prefix) {
				return path.Clean(strings.TrimPrefix(target, "./") + strings.TrimPrefix(spec, prefix)), true
			}
		}
		return "", false
	}
	err := fs.WalkDir(UI, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		var pattern *regexp.Regexp
		switch path.Ext(p) {
		case ".js":
			pattern = jsImport
		case ".css":
			pattern = cssImport
		default:
			return nil
		}
		data, err := fs.ReadFile(UI, p)
		if err != nil {
			return err
		}
		for _, m := range pattern.FindAllSubmatch(data, -1) {
			target, ok := resolve(p, string(m[1]))
			if !ok {
				t.Errorf("%s imports %s, which neither is relative nor in the import map", p, m[1])
				continue
			}
			if _, err := fs.Stat(UI, target); err != nil {
				t.Errorf("%s imports %s: %v", p, m[1], err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The policy allows the import map by its hash, so browsers apply it.
func TestPolicyAllowsImportMap(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "127.0.0.1" // without a token, only loopback hosts are served
	Handler(echo{}, "").ServeHTTP(w, r)
	if got := w.Header().Get("Content-Security-Policy"); !strings.Contains(got, "script-src 'self' 'sha256-") {
		t.Errorf("policy = %q", got)
	}
}

type echo struct{}

func (echo) ServeHTTP(w http.ResponseWriter, r *http.Request) { w.Write([]byte("api:" + r.URL.Path)) }
