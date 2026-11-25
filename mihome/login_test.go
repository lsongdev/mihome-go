package mihome

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestRefreshLoginSavesAndPreservesIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mi.token")
	client := newTestClient(t, path, "user", "old-password", func(req *http.Request) *http.Response {
		switch req.URL.Path {
		case "/pass/serviceLogin":
			if _, err := req.Cookie("passToken"); err == nil {
				t.Error("used expired pass token")
			}
			return response(200, notLoggedIn, "")
		case "/pass/serviceLoginAuth2":
			return response(200, loggedIn, "")
		default:
			return response(200, "ok", "serviceToken=new-service; Path=/")
		}
	})
	client.Token = &Tokens{UserName: "user", DeviceId: "stable", PassToken: "old-pass", Sids: map[string]SidToken{MIIO_SID: {ServiceToken: "old-service"}}, OAuth: &OAuthToken{RefreshToken: "oauth"}}
	accountURL, _ := url.Parse("https://account.xiaomi.com/")
	client.Client.Jar.SetCookies(accountURL, []*http.Cookie{{Name: "passToken", Value: "expired-cookie", Path: "/"}})
	if err := client.RefreshLogin("new-password"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved Tokens
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.DeviceId != "stable" || saved.OAuth.RefreshToken != "oauth" || saved.Sids[MIIO_SID].ServiceToken != "new-service" {
		t.Fatalf("unexpected saved credentials")
	}
	if client.Password != "new-password" {
		t.Fatal("password not retained for automatic renewal")
	}
	mode, _ := os.Stat(path)
	if mode.Mode().Perm() != 0600 {
		t.Fatal("unsafe permissions")
	}
	// A later security challenge must preserve both the working file and memory.
	client.Client.Transport = roundTripFunc(func(req *http.Request) *http.Response {
		return response(200, `&&&START&&&{"code":0,"notificationUrl":"https://account.xiaomi.com/verify"}`, "")
	})
	before := client.Token
	err = client.RefreshLogin("bad-password")
	var verification *VerificationError
	if !errors.As(err, &verification) {
		t.Fatalf("expected verification, got %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(raw) || client.Token != before || client.Password != "new-password" {
		t.Fatal("failed login replaced working credentials")
	}
}

func TestServiceTokenFromRedirectCookieJar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sts" {
			http.SetCookie(w, &http.Cookie{Name: "serviceToken", Value: "redirect-token", Path: "/"})
			http.Redirect(w, r, "/done", http.StatusFound)
		} else {
			w.Write([]byte("ok"))
		}
	}))
	defer server.Close()
	client := NewClient("user", "password")
	token, err := client.requestServiceToken(server.URL+"/sts?a=1", "sec", 1)
	if err != nil || token != "redirect-token" {
		t.Fatalf("token=%q err=%v", token, err)
	}
	parsed, _ := url.Parse(server.URL)
	if len(client.Client.Jar.Cookies(parsed)) == 0 {
		t.Fatal("test did not exercise cookie jar")
	}
}
