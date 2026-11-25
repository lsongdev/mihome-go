package mihome

import "errors"

// LoginStatus describes saved credentials, without exposing any tokens. Saved
// credentials may have expired; only a successful login verifies them upstream.
type LoginStatus struct {
	Username           string `json:"username"`
	Saved              bool   `json:"saved"`
	PasswordConfigured bool   `json:"password_configured"`
}

func (ma *Client) LoginStatus() (LoginStatus, error) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	if err := ma.ensureToken(); err != nil {
		return LoginStatus{}, err
	}
	return LoginStatus{Username: ma.Username, Saved: ma.Token.Sids[MIIO_SID].ServiceToken != "", PasswordConfigured: ma.Password != ""}, nil
}

// RefreshLogin explicitly replaces expired password-login credentials, retaining
// the device identity and the independent OAuth login. A failed attempt leaves
// the previous credentials intact. A supplied password is kept in memory only.
func (ma *Client) RefreshLogin(password string) error {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	if ma.Username == "" {
		return errors.New("请先在服务端配置 MI_USERNAME")
	}
	if err := ma.ensureToken(); err != nil {
		return err
	}
	if password == "" {
		password = ma.Password
	}
	if password == "" {
		return errors.New("请输入小米账号密码")
	}
	// Isolate the new login from expired cookies left by automatic renewal.
	// Do not replace the shared HTTP client while device requests are in flight.
	candidate := NewClient(ma.Username, password)
	candidate.Client = ma.Client
	candidate.TokenPath = ma.TokenPath
	candidate.Token = &Tokens{UserName: ma.Token.UserName, DeviceId: ma.Token.DeviceId, Sids: make(map[string]SidToken), OAuth: ma.Token.OAuth}
	if err := candidate.login(MIIO_SID); err != nil {
		return err
	}
	ma.Token, ma.Password = candidate.Token, password
	return nil
}
