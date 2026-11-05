package mihome

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const specBase = "https://miot-spec.org"

// DefaultSpecLanguage is the UI's default MIoT translation language.
const DefaultSpecLanguage = "zh_cn"

// FetchSpec returns the original specification JSON, including vendor fields.
func (c *Client) FetchSpec(ctx context.Context, urn string) (json.RawMessage, error) {
	raw, err := c.getSpecJSON(ctx, "/miot-spec-v2/instance?type="+url.QueryEscape(urn))
	if err != nil {
		return nil, fmt.Errorf("获取设备规格失败: %w", err)
	}
	var spec struct {
		Type     string            `json:"type"`
		Services []json.RawMessage `json:"services"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil || spec.Type == "" || spec.Services == nil {
		return nil, fmt.Errorf("invalid MIoT specification")
	}
	return raw, nil
}

// FetchSpecTranslations returns the original multiLanguage JSON for every
// available language. Translation selection belongs to the UI.
func (c *Client) FetchSpecTranslations(ctx context.Context, urn string) (json.RawMessage, error) {
	raw, err := c.getSpecJSON(ctx, "/instance/v2/multiLanguage?urn="+url.QueryEscape(urn))
	if err != nil {
		return nil, fmt.Errorf("获取规格翻译失败: %w", err)
	}
	var translations struct {
		Data map[string]map[string]string `json:"data"`
	}
	if err := json.Unmarshal(raw, &translations); err != nil || translations.Data == nil {
		return nil, fmt.Errorf("invalid MIoT translations")
	}
	return raw, nil
}

func (c *Client) getSpecJSON(ctx context.Context, path string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, specBase+path, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		io.Copy(io.Discard, res.Body)
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid MIoT JSON")
	}
	return json.RawMessage(raw), nil
}
