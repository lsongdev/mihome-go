package mihome

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// ensureToken makes sure ma.Token is usable, loading it from TokenPath when
// possible. A new token is saved right away, before any login attempt, so the
// device id stays the same across the runs of a security verification.
func (ma *Client) ensureToken() error {
	if ma.Token != nil {
		return nil
	}
	token, err := ma.loadToken()
	if err != nil {
		return err
	}
	if token != nil {
		ma.Token = token
		return nil
	}
	ma.Token = NewTokens()
	ma.Token.UserName = ma.Username
	ma.Token.DeviceId = strings.ToUpper(getRandom(16))
	return ma.saveToken()
}

// loadToken returns nil without an error when there is no saved token for the
// current user.
func (ma *Client) loadToken() (*Tokens, error) {
	if ma.TokenPath == "" {
		return nil, nil
	}
	content, err := os.ReadFile(ma.TokenPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 token 文件失败: %w", err)
	}
	token := NewTokens()
	if err := json.Unmarshal(content, token); err != nil {
		return nil, fmt.Errorf("解析 token 文件 %s 失败: %w", ma.TokenPath, err)
	}
	if token.UserName != ma.Username || token.DeviceId == "" {
		return nil, nil
	}
	if token.Sids == nil {
		token.Sids = make(map[string]SidToken)
	}
	return token, nil
}

func (ma *Client) saveToken() error {
	if ma.TokenPath == "" {
		return nil
	}
	content, err := json.MarshalIndent(ma.Token, "", "  ")
	if err != nil {
		return err
	}
	// The file holds login credentials: keep it readable by the owner only.
	if err := os.WriteFile(ma.TokenPath, content, 0o600); err != nil {
		return fmt.Errorf("保存 token 文件失败: %w", err)
	}
	return nil
}
