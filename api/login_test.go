package api

import (
	"errors"
	"github.com/lsongdev/mihome-go/mihome"
	"net/http"
	"strings"
	"testing"
)

type loginFake struct {
	Service
	err      error
	password string
}

func (f *loginFake) LoginStatus() (mihome.LoginStatus, error) {
	return mihome.LoginStatus{Username: "user", Saved: true, PasswordConfigured: true}, nil
}
func (f *loginFake) RefreshLogin(password string) error { f.password = password; return f.err }

func TestAccountLoginAPI(t *testing.T) {
	f := &loginFake{}
	s := New(f, Options{})
	status := call(s, "GET", "/login", "")
	if status.Code != 200 || status.Header().Get("Cache-Control") != "no-store" || strings.Contains(status.Body.String(), "service_Token") {
		t.Fatal(status.Body.String())
	}
	w := call(s, "POST", "/login", `{"password":"secret"}`)
	if w.Code != 200 || f.password != "secret" || strings.Contains(w.Body.String(), "secret") {
		t.Fatal(w.Body.String())
	}
	f.err = &mihome.VerificationError{URL: "https://account.xiaomi.com/verify"}
	w = call(s, "POST", "/login", `{}`)
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "verification_required") {
		t.Fatal(w.Body.String())
	}
	f.err = errors.New("登录失败")
	if w = call(s, "POST", "/login", `{}`); w.Code != http.StatusBadGateway {
		t.Fatal(w.Code)
	}
	if w = call(s, "POST", "/login", `{"token":"not-accepted"}`); w.Code != http.StatusBadRequest {
		t.Fatal(w.Code)
	}
}
