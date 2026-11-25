package api

import (
	"errors"
	"github.com/lsongdev/mihome-go/mihome"
	"net/http"
)

type accountLogin interface {
	LoginStatus() (mihome.LoginStatus, error)
	RefreshLogin(password string) error
}

func (s *Server) loginStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	account, ok := s.service.(accountLogin)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("账号登录不可用"))
		return
	}
	status, err := account.LoginStatus()
	respond(w, status, err)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var input struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &input) {
		return
	}
	account, ok := s.service.(accountLogin)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("账号登录不可用"))
		return
	}
	if err := account.RefreshLogin(input.Password); err != nil {
		writeUpstreamError(w, err)
		return
	}
	select {
	case s.authorized <- struct{}{}:
	default:
	}
	writeJSON(w, http.StatusOK, map[string]bool{"saved": true})
}
