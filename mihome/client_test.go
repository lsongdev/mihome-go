package mihome

import (
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type roundTripFunc func(*http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func response(status int, body string, setCookie string) *http.Response {
	header := http.Header{}
	if setCookie != "" {
		header.Set("Set-Cookie", setCookie)
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newTestClient(t *testing.T, path, user, password string, f roundTripFunc) *Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(user, password)
	client.TokenPath = path
	client.Client = &http.Client{Jar: jar, Transport: f}
	return client
}

const (
	notLoggedIn = `&&&START&&&{"code":70016,"desc":"登录验证失败","qs":"q","sid":"micoapi","_sign":"s","callback":"c"}`
	loggedIn    = `&&&START&&&{"code":0,"userId":1,"passToken":"pass","ssecurity":"sec","nonce":1,"location":"https://sts.example/sts?a=1"}`
)

func TestLoginWithoutCredentialsFailsBeforeSubmitting(t *testing.T) {
	calls := 0
	client := newTestClient(t, "", "", "", func(req *http.Request) *http.Response {
		calls++
		return response(200, notLoggedIn, "")
	})
	err := client.Login("micoapi")
	if err == nil || !strings.Contains(err.Error(), "未设置账号或密码") {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("password step should not run, got %d calls", calls)
	}
}

func TestDeviceIdIsKeptAcrossFailedLogins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	var deviceIds []string
	verify := func(req *http.Request) *http.Response {
		if req.Method == http.MethodGet {
			cookie, err := req.Cookie("deviceId")
			if err != nil {
				t.Fatal(err)
			}
			deviceIds = append(deviceIds, cookie.Value)
			return response(200, notLoggedIn, "")
		}
		return response(200, `&&&START&&&{"code":0,"notificationUrl":"https://verify.example"}`, "")
	}
	for i := 0; i < 2; i++ {
		err := newTestClient(t, path, "user", "password", verify).Login("micoapi")
		if err == nil || !strings.Contains(err.Error(), "https://verify.example") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if len(deviceIds) != 2 || deviceIds[0] != deviceIds[1] {
		t.Fatalf("device id changed between runs: %v", deviceIds)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("token file mode = %o", info.Mode().Perm())
	}
}

func TestSavedTokenOfAnotherUserIsIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	first := newTestClient(t, path, "alice", "x", nil)
	if err := first.ensureToken(); err != nil {
		t.Fatal(err)
	}
	second := newTestClient(t, path, "bob", "x", nil)
	if err := second.ensureToken(); err != nil {
		t.Fatal(err)
	}
	if second.Token.UserName != "bob" || second.Token.DeviceId == first.Token.DeviceId {
		t.Fatalf("reused token of another user: %+v", second.Token)
	}
}

func TestRequestLogsInAgainWhenSavedTokenExpired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	var apiCookies []string
	client := newTestClient(t, path, "user", "password", func(req *http.Request) *http.Response {
		switch req.URL.Host {
		case "account.xiaomi.com":
			return response(200, loggedIn, "")
		case "sts.example":
			return response(200, "", "serviceToken=fresh")
		}
		apiCookies = append(apiCookies, req.Header.Get("Cookie"))
		if len(apiCookies) == 1 {
			return response(401, "unauthorized", "")
		}
		return response(200, `{"code":0,"data":"ok"}`, "")
	})
	client.Token = NewTokens()
	client.Token.Sids["micoapi"] = SidToken{ServiceToken: "stale"}

	out := H{}
	headers := http.Header{"User-Agent": {UA}}
	err := client.Request("micoapi", "https://api.example/x", nil, nil, headers, out)
	if err != nil {
		t.Fatal(err)
	}
	if out["data"] != "ok" || len(apiCookies) != 2 {
		t.Fatalf("out = %v, calls = %d", out, len(apiCookies))
	}
	if !strings.Contains(apiCookies[1], "serviceToken=fresh") || strings.Contains(apiCookies[1], "stale") {
		t.Fatalf("retry sent wrong cookies: %q", apiCookies[1])
	}
	if headers.Get("Cookie") != "" {
		t.Fatalf("caller headers were modified: %v", headers)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), "fresh") {
		t.Fatalf("new token was not saved: %s", saved)
	}
}

func TestRequestDoesNotRetryForever(t *testing.T) {
	apiCalls := 0
	client := newTestClient(t, "", "user", "password", func(req *http.Request) *http.Response {
		switch req.URL.Host {
		case "account.xiaomi.com":
			return response(200, loggedIn, "")
		case "sts.example":
			return response(200, "", "serviceToken=fresh")
		}
		apiCalls++
		return response(401, "unauthorized", "")
	})
	err := client.Request("micoapi", "https://api.example/x", nil, nil, nil, H{})
	if err == nil || apiCalls != 2 {
		t.Fatalf("err = %v, api calls = %d", err, apiCalls)
	}
}

func TestRequestReportsResponseWithoutCode(t *testing.T) {
	client := newTestClient(t, "", "user", "password", func(req *http.Request) *http.Response {
		return response(200, `{"message":"no code"}`, "")
	})
	client.Token = NewTokens()
	client.Token.Sids["micoapi"] = SidToken{ServiceToken: "t"}
	err := client.Request("micoapi", "https://api.example/x", nil, nil, nil, H{})
	if err == nil || !strings.Contains(err.Error(), "code") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Run with -race: concurrent requests share the token, and all of them finding
// it expired must cause only one login.
func TestConcurrentRequestsLogInOnce(t *testing.T) {
	var mu sync.Mutex
	logins := 0
	client := newTestClient(t, "", "user", "password", func(req *http.Request) *http.Response {
		switch req.URL.Host {
		case "account.xiaomi.com":
			mu.Lock()
			logins++
			mu.Unlock()
			return response(200, loggedIn, "")
		case "sts.example":
			return response(200, "", "serviceToken=fresh")
		}
		if strings.Contains(req.Header.Get("Cookie"), "stale") {
			return response(401, "unauthorized", "")
		}
		return response(200, `{"code":0}`, "")
	})
	client.Token = NewTokens()
	client.Token.Sids["micoapi"] = SidToken{ServiceToken: "stale"}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := client.Request("micoapi", "https://api.example/x", nil, nil, nil, H{}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if logins != 1 {
		t.Fatalf("logged in %d times", logins)
	}
}

func TestVerificationErrorCarriesTheURL(t *testing.T) {
	client := newTestClient(t, "", "user", "password", func(req *http.Request) *http.Response {
		if req.Method == http.MethodGet {
			return response(200, notLoggedIn, "")
		}
		return response(200, `&&&START&&&{"code":0,"notificationUrl":"https://verify.example"}`, "")
	})
	var verification *VerificationError
	if err := client.Login("micoapi"); !errors.As(err, &verification) || verification.URL != "https://verify.example" {
		t.Fatalf("unexpected error: %v", err)
	}
}
