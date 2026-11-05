package mihome

import (
	"encoding/json"
	"net/http"
	"net/url"
)

const MIIO_SID = "xiaomiio"

func (s *Client) MiioRequest(path string, data any) (any, error) {
	prepareData := func(token *Tokens, cookies map[string]string) url.Values {
		cookies["PassportDeviceId"] = token.DeviceId
		return signData(path, data, token.Sids[MIIO_SID].Ssecurity)
	}
	headers := http.Header{
		"User-Agent":                 []string{UA},
		"x-xiaomi-protocal-flag-cli": []string{"PROTOCAL-HTTP2"},
	}
	var resp H = make(map[string]any)
	err := s.Request(MIIO_SID, "https://api.io.mi.com/app"+path, nil, prepareData, headers, resp)
	if err != nil {
		return nil, err
	}
	return resp["result"], nil
}

type DeviceInfo struct {
	Name  string `json:"name"`
	Model string `json:"model"`
	Did   string `json:"did"`
	Token string `json:"token"`
	// IsOnline is only reported by HomeDevices.
	IsOnline bool `json:"isOnline"`
	// SpecType is the URN of the device's MIoT specification, if it has one. It
	// is only reported by HomeDevices.
	SpecType string `json:"spec_type"`
}

type DeviceListFilter struct {
	ShowVirtualModel bool `json:"getVirtualModel"`
	ShowHuamiDevices int  `json:"getHuamiDevices"`
}

func (s *Client) HomeDeviceList(filter *DeviceListFilter) (devices []DeviceInfo, err error) {
	result, err := s.MiioRequest("/home/device_list", filter)
	if err != nil {
		return nil, err
	}
	deviceList := result.(H)["list"].([]any)
	devices = make([]DeviceInfo, len(deviceList))
	for i, item := range deviceList {
		ja, _ := json.Marshal(item)
		json.Unmarshal(ja, &devices[i])
	}
	return
}

// Home is a Mi Home family, either owned by the account or shared with it.
type Home struct {
	ID     json.Number `json:"id"`
	Name   string      `json:"name"`
	Owner  json.Number `json:"uid"`
	Shared bool        `json:"-"`
}

// decodeResult converts a generic API result into a typed value.
func decodeResult(result, out any) error {
	content, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return json.Unmarshal(content, out)
}

// HomeList returns the homes owned by the account and the ones shared with it.
func (s *Client) HomeList() ([]Home, error) {
	result, err := s.MiioRequest("/v2/homeroom/gethome", H{
		"fg":              true,
		"fetch_share":     true,
		"fetch_share_dev": true,
		"limit":           300,
		"app_ver":         7,
	})
	if err != nil {
		return nil, err
	}
	var res struct {
		Owned  []Home `json:"homelist"`
		Shared []Home `json:"share_home_list"`
	}
	if err := decodeResult(result, &res); err != nil {
		return nil, err
	}
	for i := range res.Shared {
		res.Shared[i].Shared = true
	}
	return append(res.Owned, res.Shared...), nil
}

// HomeDevices returns all the devices of a home. Unlike HomeDeviceList, which
// only covers the devices owned by the account, it also works on shared homes.
func (s *Client) HomeDevices(home Home) ([]DeviceInfo, error) {
	var devices []DeviceInfo
	startDid := ""
	for {
		result, err := s.MiioRequest("/v2/home/home_device_list", H{
			"home_owner":         home.Owner,
			"home_id":            home.ID,
			"limit":              200,
			"start_did":          startDid,
			"get_split_device":   false,
			"support_smart_home": true,
			"get_cariot_device":  true,
			"get_third_device":   true,
		})
		if err != nil {
			return nil, err
		}
		var page struct {
			Devices []DeviceInfo `json:"device_info"`
			HasMore bool         `json:"has_more"`
			MaxDid  string       `json:"max_did"`
		}
		if err := decodeResult(result, &page); err != nil {
			return nil, err
		}
		devices = append(devices, page.Devices...)
		if !page.HasMore || page.MaxDid == "" {
			return devices, nil
		}
		startDid = page.MaxDid
	}
}

func (s *Client) HomeRequest(did, method string, params any) (H, error) {
	resp, err := s.MiioRequest("/home/rpc/"+did, H{
		"accessKey": "IOS00026747c5acafc2",
		"id":        1,
		"method":    method,
		"params":    params,
	})
	if err != nil {
		return nil, err
	}
	return resp.(H), nil
}

func (s *Client) HomeGetProps(did string, props []string) (H, error) {
	return s.HomeRequest(did, "get_prop", props)
}

func (s *Client) HomeSetProps(did string, props H) (map[string]int, error) {
	results := make(map[string]int, len(props))
	for prop, value := range props {
		result, err := s.HomeSetProp(did, prop, value)
		if err != nil {
			return nil, err
		}
		results[prop] = result
	}
	return results, nil
}

func (s *Client) HomeGetProp(did, prop string) (any, error) {
	results, err := s.HomeGetProps(did, []string{prop})
	if err != nil {
		return nil, err
	}
	return results[prop], nil
}

func (s *Client) HomeSetProp(did, prop string, value any) (int, error) {
	result, err := s.HomeRequest(did, "set_"+prop, value)
	if err != nil {
		return 0, err
	}
	if result["result"] == "ok" {
		return 0, nil
	}
	return -1, nil
}
