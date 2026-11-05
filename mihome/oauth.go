package mihome

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	oauthAuthorizeURL = "https://account.xiaomi.com/oauth2/authorize"
	oauthTokenURL     = "https://ha.api.io.mi.com/app/v2/ha/oauth/get_token"
	oauthRedirectBase = "http://homeassistant.local:8123/api/webhook/"
	oauthUserAgent    = "ha_xiaomi_home/v0.5.0 Linux client/"
	// Tokens are renewed early, as the official integration does.
	oauthLifetimeRatio = 0.7
)

// ErrOAuthRequired means a browser login is needed: see OAuthURL.
var ErrOAuthRequired = errors.New("需要登录小米账号以接收实时事件")

// OAuthToken is the login for realtime events, kept with the other tokens. Events
// need this separate OAuth login, unlike the password login of the other requests.
type OAuthToken struct {
	UUID         string    `json:"uuid"`
	VirtualDid   string    `json:"virtual_did"`
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	RenewAt      time.Time `json:"renew_at"`
}

func (t *OAuthToken) deviceID() string { return "ha." + t.UUID }

func (t *OAuthToken) redirectURL() string { return oauthRedirectBase + t.VirtualDid }

func (t *OAuthToken) state() string {
	sum := sha1.Sum([]byte("d=" + t.deviceID()))
	return hex.EncodeToString(sum[:])
}

// OAuthReady reports whether the login for realtime events has been done.
func (ma *Client) OAuthReady() bool {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	if err := ma.ensureToken(); err != nil {
		return false
	}
	return ma.Token.OAuth != nil && ma.Token.OAuth.RefreshToken != ""
}

// OAuthURL returns the page where the user logs in. It then redirects to a local
// address that does not exist: pass the address it tried to open to CompleteOAuth.
func (ma *Client) OAuthURL() (string, error) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	if ma.OAuthClientID == "" {
		return "", errors.New("未配置 OAuth client ID")
	}
	if err := ma.ensureToken(); err != nil {
		return "", err
	}
	if ma.Token.OAuth == nil {
		token, err := newOAuthToken()
		if err != nil {
			return "", err
		}
		ma.Token.OAuth = token
		if err := ma.saveToken(); err != nil {
			return "", err
		}
	}
	token := ma.Token.OAuth
	return oauthAuthorizeURL + "?" + url.Values{
		"redirect_uri":  {token.redirectURL()},
		"client_id":     {ma.OAuthClientID},
		"response_type": {"code"},
		"device_id":     {token.deviceID()},
		"state":         {token.state()},
		"skip_confirm":  {"False"},
	}.Encode(), nil
}

func newOAuthToken() (*OAuthToken, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	virtual, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 64))
	if err != nil {
		return nil, err
	}
	return &OAuthToken{UUID: hex.EncodeToString(id), VirtualDid: virtual.String()}, nil
}

// CompleteOAuth finishes the login, given the address the login page redirected
// to. Its state must match, so a code from elsewhere is refused.
func (ma *Client) CompleteOAuth(redirected string) error {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	if err := ma.ensureToken(); err != nil {
		return err
	}
	token := ma.Token.OAuth
	if token == nil {
		return errors.New("请先打开登录页面")
	}
	parsed, err := url.Parse(strings.TrimSpace(redirected))
	if err != nil || parsed.Query().Get("code") == "" {
		return errors.New("地址里没有授权码，请粘贴跳转后地址栏的完整地址")
	}
	if parsed.Query().Get("state") != token.state() {
		return errors.New("授权地址不属于这次登录，请重新打开登录页面")
	}
	return ma.requestOAuthToken(token, map[string]any{
		"code":      parsed.Query().Get("code"),
		"device_id": token.deviceID(),
	})
}

// oauthAccessToken returns a valid access token, renewing it if needed.
func (ma *Client) oauthAccessToken() (string, *OAuthToken, error) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	if err := ma.ensureToken(); err != nil {
		return "", nil, err
	}
	token := ma.Token.OAuth
	if token == nil || token.RefreshToken == "" {
		return "", nil, ErrOAuthRequired
	}
	if time.Now().After(token.RenewAt) {
		err := ma.requestOAuthToken(token, map[string]any{"refresh_token": token.RefreshToken})
		if err != nil {
			return "", nil, err
		}
	}
	return token.AccessToken, token, nil
}

// requestOAuthToken must be called with mu held.
func (ma *Client) requestOAuthToken(token *OAuthToken, data map[string]any) error {
	data["client_id"] = json.Number(ma.OAuthClientID)
	data["redirect_uri"] = token.redirectURL()
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodGet, oauthTokenURL+"?"+url.Values{"data": {string(encoded)}}.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", oauthUserAgent+token.deviceID())
	res, err := ma.Client.Do(req)
	if err != nil {
		// The request URL holds the token or code, so keep only the cause.
		var failure *url.Error
		if errors.As(err, &failure) {
			err = failure.Err
		}
		return fmt.Errorf("请求 OAuth token 失败: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	var answer struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Result  struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int64  `json:"expires_in"`
		} `json:"result"`
	}
	_ = json.Unmarshal(body, &answer)
	if res.StatusCode != http.StatusOK || answer.Code != 0 || answer.Result.AccessToken == "" || answer.Result.RefreshToken == "" {
		return fmt.Errorf("获取 OAuth token 失败: HTTP %d, code %d, %s", res.StatusCode, answer.Code, answer.Message)
	}
	token.AccessToken = answer.Result.AccessToken
	token.RefreshToken = answer.Result.RefreshToken
	lifetime := time.Duration(float64(answer.Result.ExpiresIn)*oauthLifetimeRatio) * time.Second
	token.RenewAt = time.Now().Add(lifetime)
	return ma.saveToken()
}
