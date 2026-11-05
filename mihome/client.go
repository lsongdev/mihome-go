package mihome

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

type H = map[string]any

const UA = "MiHome/6.0.103 (com.xiaomi.mihome; build:6.0.103.1; iOS 14.4.0) Alamofire/6.0.103 MICO/iOSApp/appStore/6.0.103"

type SidToken struct {
	Ssecurity    string `json:"ssecurity"`
	ServiceToken string `json:"service_Token"`
}

type Tokens struct {
	UserName  string              `json:"user_name"`
	DeviceId  string              `json:"device_id"`
	UserId    string              `json:"user_id"`
	PassToken string              `json:"pass_Token"`
	Sids      map[string]SidToken `json:"sids"`
	OAuth     *OAuthToken         `json:"oauth,omitempty"`
}

func NewTokens() *Tokens {
	return &Tokens{
		Sids: make(map[string]SidToken),
	}
}

type Client struct {
	Token    *Tokens
	Client   *http.Client
	Username string
	Password string
	// TokenPath is where the device id and tokens are kept between runs.
	// Leave it empty to keep them in memory only.
	TokenPath string
	// OAuthClientID identifies the OAuth login that realtime events need.
	OAuthClientID string

	// mu guards Token, so a Client can be shared between goroutines. It is
	// held during a login: concurrent requests wait for the one login.
	mu sync.Mutex
}

// VerificationError means Xiaomi wants the account verified in a browser
// before it allows this login.
type VerificationError struct{ URL string }

func (e *VerificationError) Error() string {
	return "需要安全验证，请在浏览器中打开以下链接完成验证后重试: " + e.URL
}

func NewClient(username string, password string) *Client {
	j, _ := cookiejar.New(nil)
	return &Client{
		Client: &http.Client{
			Jar:     j,
			Timeout: 30 * time.Second,
		},
		Username: username,
		Password: password,
	}
}

/*
	&&&START&&&{
	    "serviceParam": "{\"checkSafePhone\":false,\"checkSafeAddress\":false,\"lsrp_score\":0.0}",
	    "qs": "%3Fsid%3Dmicoapi%26_json%3Dtrue",
	    "code": 70016,
	    "description": "登录验证失败",
	    "securityStatus": 0,
	    "_sign": "58VY7HzpBxBcLsgTGFWDyJ7fEtw=",
	    "sid": "micoapi",
	    "result": "error",
	    "miDemo": 0,
	    "captchaUrl": null,
	    "callback": "https://api2.mina.mi.com/sts",
	    "location": "https://account.xiaomi.com/fe/service/login?_json=true&sid=micoapi&qs=%253Fsid%253Dmicoapi%2526_json%253Dtrue&callback=https%3A%2F%2Fapi2.mina.mi.com%2Fsts&_sign=58VY7HzpBxBcLsgTGFWDyJ7fEtw%3D&serviceParam=%7B%22checkSafePhone%22%3Afalse%2C%22checkSafeAddress%22%3Afalse%2C%22lsrp_score%22%3A0.0%7D&showActiveX=false&theme=&needTheme=false&bizDeviceType=",
	    "pwd": 0,
	    "child": 0,
	    "desc": "登录验证失败"
	}
*/
/*
	&&&START&&&{
		"notificationUrl": "https://account.xiaomi.com/fe/service/identity/authStart?mask=4294967296&sid=micoapi&context=j06JTKj5NAJbEQN388a-lVn4jW6G-P3y4z2Dm6mp-Y-q4j4AWudl96OH885cjLXM0EeIWY80GRwW_ELaamIeA4XyFF-pezBKDjA1FTyff0UJLnXTcN_YocgOO7y-wfqJOPsIX33auHINz3fSrQ8pwYtP08LGpvat2t0wc--Jsliga3S7UKBmcMBlnxS-JaUMIGwPWQSVk7Zy3mlDNuLb6kcokakJDqM8ozMgsAh0Xpcm7i5jb4Ij9nZrtGTSJSbD6B01es-V79e8Ijq1vp8BSoGDATBbCvKBQKVmpR4Qiz_Tu9zT7-uhXMHUVeH0aqfSG9_wLdk1XrTmDV0R5Qec2OJkmOvV8lMumlSU2JYhtKmzzEAivotCQTYeqXEWU5fBMjHfIKO_OgToGXKOeG9vyx4Tbe5a4tFsNThbG8wHWj5iaKXjrEs4T-oj7qNsMVObdcRj8DZxHxXpXfrFuAoq24Iw37sCuEkWxyJGJVS4C7qzqhMHOdOoC5yRI2mobJQDldymALYMXCoT2oOfEJ1sV_c3B2Moju-t5fvse3XFTEvCFWeqmXVgpwaSZabFstJvp1ln3BVp1fdI3XFRKncWhMPnaA5vCpEE6C5vOwK5JwXkdoLCdX0gaNkQV-J36MDuSyCekPAa4pdyJvJaf1FuuFQMZ5QDyCba-1gJwrhgqMJ7IEY6clHxMXCpk0428dEovU0Qq0FKcA6-jZjvWzdotZWU0XWC8SjrHzAIr0ak8Ul8fakopxFFRWVrZE3CrutkXpg0IwafuU288cuR5D9VBiy43Gz37lygtY3LXGdsvNQ&_locale=zh_CN",
		"result": "ok",
		"code": 0,
		"miDemo": 0,
		"captchaUrl": null,
		"description": "成功",
		"location": "",
		"securityStatus": 16,
		"pwd": 0,
		"child": 0,
		"desc": "成功"
	}
	&&&START&&&{
		"qs": "%3Fsid%3Dmicoapi%26_json%3Dtrue",
		"ssecurity": "GzQp36CUTLlxjhu857oqAQ==",
		"code": 0,
		"passToken": "V1:DXmurwq2/R1BHTELu6obCe7IGCsy2ygh/0/HHWxGX1n+Fsvswwz79pexDWs6aSTnoKso98amCR/T/49Ej8cnR47+iegXPoi8JSHVE5gOG6Hr0gHFsqHOBy4fqI0n4eTxwMqxbVtYcalF4N/eYW1PHOAcFZhGAlmaXHvA2/dWQ2mhhfCDRk8BmGnMUYbkvNbhRDKhHg6LgOJ331n/sdROXMDw9Bwq9D9+zOSSNlFl0XyzON2szh+6rKPTyCf3rjZrHM43ZZT+enVw2IWwsUIy+cnixljh3gfBbvuC8C+nMyWdJa49IR1XdYy1tuKglQ6CAxCQCxJobMtVlKfFowWpfw==",
		"description": "成功",
		"securityStatus": 0,
		"nonce": 6444317629414392832,
		"userId": 833175247,
		"cUserId": "bJAwGb4agKjis8KHgNq-ve5CSi8",
		"result": "ok",
		"psecurity": "S5gf5eNLng2SFJWP2ClHTQ==",
		"miDemo": 0,
		"captchaUrl": null,
		"location": "https://api2.mina.mi.com/sts?d=ARMPSJCLYJI97DIY&ticket=0&pwd=1&p_ts=1773724859214&fid=0&p_lm=1&p_ur=US&auth=DajFn2eEqxo5KbQAZ91nqA%2FIVnkAqMVUTSQnvNzbWAIVdSrzlKs5YKqzqLw87XP0gZ3i7IfqeDeqtdG4S5DfnN4ZMQVSKqgLygchE1raYhs8vkRuNLt3isOKoYAAf8grRakt6W6i3dFtCj%2FF7OS%2Bzyv8i9tnh68z%2BNXKm3dMGp8%3D&m=1&_group=DEFAULT&tsl=0&p_ca=0&p_idc=China&nonce=W8u1QjZAoKkBwxTg&_ssign=lMU3LlkUKAA9wb1sjRJvPFLKXnI%3D",
		"pwd": 1,
		"child": 0,
		"desc": "成功"
	}
*/
type loginResp struct {
	Qs              string `json:"qs"`
	Ssecurity       string `json:"ssecurity"`
	Code            int    `json:"code"`
	PassToken       string `json:"passToken"`
	Description     string `json:"description"`
	SecurityStatus  int    `json:"securityStatus"`
	Nonce           int64  `json:"nonce"`
	UserID          int    `json:"userId"`
	CUserID         string `json:"cUserId"`
	Result          string `json:"result"`
	PSecurity       string `json:"psecurity"`
	CaptchaURL      any    `json:"captchaUrl"`
	NotificationURL string `json:"notificationUrl"`
	Location        string `json:"location"`
	Pwd             int    `json:"pwd"`
	Child           int    `json:"child"`
	Desc            string `json:"desc"`
	ServiceParam    string `json:"serviceParam"`
	Sign            string `json:"_sign"`
	Sid             string `json:"sid"`
	Callback        string `json:"callback"`
}

// sid: service id, like "xiaomiio", "micoapi", "mina"
func (ma *Client) Login(sid string) error {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	return ma.login(sid)
}

// AccountToken returns the account's user id and pass token, logging in if there
// are none yet. Other clients of the account can log in with them.
func (ma *Client) AccountToken() (userID, passToken string, err error) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	if err := ma.ensureToken(); err != nil {
		return "", "", err
	}
	if ma.Token.PassToken == "" {
		if err := ma.login(MIIO_SID); err != nil {
			return "", "", err
		}
	}
	return ma.Token.UserId, ma.Token.PassToken, nil
}

// login must be called with mu held.
func (ma *Client) login(sid string) error {
	err := ma.ensureToken()
	if err != nil {
		return err
	}
	cookies := []*http.Cookie{
		{Name: "sdkVersion", Value: "3.9"},
		{Name: "deviceId", Value: ma.Token.DeviceId},
	}
	if ma.Token.PassToken != "" {
		cookies = append(cookies, &http.Cookie{Name: "userId", Value: ma.Token.UserId})
		cookies = append(cookies, &http.Cookie{Name: "passToken", Value: ma.Token.PassToken})
	}
	var resp *loginResp
	resp, err = ma.serviceLogin(fmt.Sprintf("serviceLogin?sid=%s&_json=true", sid), nil, cookies)
	if err != nil {
		return err
	}
	// A non-zero code here is the normal "not logged in yet" answer (70016):
	// it carries the _sign and callback needed to submit the password.
	if resp.Code != 0 {
		if ma.Username == "" || ma.Password == "" {
			return errors.New("登录失败: 未设置账号或密码")
		}
		data := url.Values{
			"_json":    {"true"},
			"qs":       {resp.Qs},
			"sid":      {resp.Sid},
			"_sign":    {resp.Sign},
			"callback": {resp.Callback},
			"user":     {ma.Username},
			"hash":     {strings.ToUpper(fmt.Sprintf("%x", md5.Sum([]byte(ma.Password))))},
		}
		resp, err = ma.serviceLogin("serviceLoginAuth2", data, cookies)
		if err != nil {
			return err
		}
		if resp.Code != 0 {
			return fmt.Errorf("登录失败: %s (code: %d)", resp.Desc, resp.Code)
		}
	}
	if resp.NotificationURL != "" {
		return &VerificationError{URL: resp.NotificationURL}
	}
	ma.Token.UserId = fmt.Sprint(resp.UserID)
	ma.Token.PassToken = resp.PassToken
	serviceToken, err := ma.requestServiceToken(resp.Location, resp.Ssecurity, resp.Nonce)
	if err != nil {
		return err
	}
	ma.Token.Sids[sid] = SidToken{
		Ssecurity:    resp.Ssecurity,
		ServiceToken: serviceToken,
	}
	return ma.saveToken()
}

func (ma *Client) serviceLogin(name string, data url.Values, cookies []*http.Cookie) (*loginResp, error) {
	headers := http.Header{
		"User-Agent": []string{UA},
	}
	var reqBody io.Reader
	method := http.MethodGet
	if data != nil {
		reqBody = strings.NewReader(data.Encode())
		method = http.MethodPost
		headers.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req, _ := http.NewRequest(method, "https://account.xiaomi.com/pass/"+name, reqBody)
	req.Header = headers
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	resp, err := ma.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var jsonResponse loginResp
	err = json.Unmarshal(bytes.TrimPrefix(body, []byte("&&&START&&&")), &jsonResponse)
	if err != nil {
		return nil, fmt.Errorf("解析登录响应失败 (HTTP %d): %w", resp.StatusCode, err)
	}
	return &jsonResponse, nil
}

func (ma *Client) requestServiceToken(location, ssecurity string, nonce int64) (string, error) {
	nsec := fmt.Sprintf("nonce=%d&%s", nonce, ssecurity)
	sum := sha1.Sum([]byte(nsec))
	clientSign := base64.StdEncoding.EncodeToString(sum[:])
	es := url.QueryEscape(clientSign)
	requestUrl := fmt.Sprintf("%s&clientSign=%s", location, es)
	req, _ := http.NewRequest(http.MethodGet, requestUrl, nil)
	headers := http.Header{
		"User-Agent": []string{UA},
	}
	req.Header = headers
	resp, err := ma.Client.Do(req)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("response error: %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	cookies := resp.Cookies()
	var serviceToken string
	for _, cookie := range cookies {
		if cookie.Name == "serviceToken" {
			serviceToken = cookie.Value
			break
		}
	}
	body, _ := io.ReadAll(resp.Body)
	if serviceToken == "" {
		return "", fmt.Errorf("service token not found in cookie, body: %s", body)
	}
	return serviceToken, nil
}

type DataCb func(Tokens *Tokens, cookie map[string]string) url.Values

func (ma *Client) NewRequest(sid, u string, data url.Values, cb DataCb, headers http.Header) *http.Request {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	var req *http.Request
	var body io.Reader
	// Work on a copy: AddCookie writes into the header, and a retry reuses the
	// caller's headers, which must not keep the expired cookies.
	headers = headers.Clone()
	if headers == nil {
		headers = http.Header{}
	}
	cookies := []*http.Cookie{
		{Name: "userId", Value: ma.Token.UserId},
		{Name: "serviceToken", Value: ma.Token.Sids[sid].ServiceToken},
	}
	method := http.MethodGet
	if data != nil || cb != nil {
		var vals url.Values
		if cb != nil {
			var cookieMap = make(map[string]string)
			vals = cb(ma.Token, cookieMap)
			for k, v := range cookieMap {
				cookies = append(cookies, &http.Cookie{Name: k, Value: v})
			}
		} else if data != nil {
			vals = data
		}
		if vals != nil {
			method = http.MethodPost
			// log.Println("request data", vals.Encode())
			body = strings.NewReader(vals.Encode())
			headers.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	}
	req, _ = http.NewRequest(method, u, body)
	req.Header = headers
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	return req
}

func (ma *Client) hasSid(sid string) bool {
	if ma.Token == nil {
		return false
	}
	_, ok := ma.Token.Sids[sid]
	return ok
}

// errTokenExpired means the saved serviceToken was rejected by the server.
var errTokenExpired = errors.New("serviceToken 已失效")

func (ma *Client) Request(sid, u string, data url.Values, cb DataCb, headers http.Header, output H) error {
	used, err := ma.sidToken(sid)
	if err != nil {
		return err
	}
	err = ma.doRequest(sid, u, data, cb, headers, output)
	if !errors.Is(err, errTokenExpired) {
		return err
	}
	// Tokens loaded from disk can expire between runs: log in again once.
	if err := ma.relogin(sid, used); err != nil {
		return err
	}
	return ma.doRequest(sid, u, data, cb, headers, output)
}

// sidToken returns the service token of sid, logging in if there is none.
func (ma *Client) sidToken(sid string) (string, error) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	if err := ma.ensureToken(); err != nil {
		return "", err
	}
	if !ma.hasSid(sid) {
		if err := ma.login(sid); err != nil {
			return "", err
		}
	}
	return ma.Token.Sids[sid].ServiceToken, nil
}

// relogin replaces a rejected service token, unless a concurrent request has
// already done it.
func (ma *Client) relogin(sid, rejected string) error {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	if ma.Token.Sids[sid].ServiceToken != rejected {
		return nil
	}
	delete(ma.Token.Sids, sid)
	return ma.login(sid)
}

func (ma *Client) doRequest(sid, u string, data url.Values, cb DataCb, headers http.Header, output H) error {
	req := ma.NewRequest(sid, u, data, cb, headers)
	resp, err := ma.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return errTokenExpired
	}
	err = json.NewDecoder(resp.Body).Decode(&output)
	if err != nil {
		return fmt.Errorf("解析响应失败 (HTTP %d): %w", resp.StatusCode, err)
	}
	code, ok := output["code"].(float64)
	if !ok {
		return fmt.Errorf("响应缺少 code 字段 (HTTP %d)", resp.StatusCode)
	}
	if code == 0 {
		return nil
	}
	message := fmt.Sprint(output["message"])
	if strings.Contains(strings.ToLower(message), "auth") {
		return errTokenExpired
	}
	return fmt.Errorf("error: %s (code: %.0f)", message, code)
}
