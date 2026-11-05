package mihome

import (
	"errors"
	"fmt"
)

func (s *Client) MiotRequest(cmd string, params any) (any, error) {
	return s.MiioRequest("/miotspec/"+cmd, H{"params": params})
}

type Iid struct {
	Siid int `json:"siid"`
	Piid int `json:"piid"`
}

// miotPropsLimit is how many properties MiotGetProps asks for in one request.
const miotPropsLimit = 30

// MiotGetProps reads properties, in as many requests as needed. Each result has
// its own code: a device can fail on some properties only.
func (s *Client) MiotGetProps(did string, iids []Iid) ([]any, error) {
	results := make([]any, 0, len(iids))
	for len(iids) > 0 {
		n := min(len(iids), miotPropsLimit)
		params := make([]H, n)
		for i, iid := range iids[:n] {
			params[i] = H{
				"did":  did,
				"siid": iid.Siid,
				"piid": iid.Piid,
			}
		}
		resp, err := s.MiotRequest("prop/get", params)
		if err != nil {
			return nil, err
		}
		page, ok := resp.([]any)
		if !ok {
			return nil, fmt.Errorf("unexpected prop/get result: %v", resp)
		}
		results = append(results, page...)
		iids = iids[n:]
	}
	return results, nil
}

func (s *Client) MiotSetProps(did string, props map[Iid]any) (any, error) {
	params := make([]H, len(props))
	index := 0
	for i, prop := range props {
		params[index] = H{
			"did":   did,
			"siid":  i.Siid,
			"piid":  i.Piid,
			"value": prop,
		}
		index++
	}
	return s.MiotRequest("prop/set", params)
}

func (s *Client) MiotGetProp(did string, iid Iid) (H, error) {
	results, err := s.MiotGetProps(did, []Iid{iid})
	if err != nil {
		return nil, err
	}
	result := results[0].(H)
	if result == nil {
		return nil, errors.New("no result")
	}
	if result["code"].(float64) != 0 {
		return nil, fmt.Errorf("error code: %2f", result["code"])
	}
	return result, nil
}

func (s *Client) MiotSetProp(did string, iid Iid, value any) (any, error) {
	return s.MiotSetProps(did, map[Iid]any{iid: value})
}

func (s *Client) MiotAction(did string, iid []int, in []any) (H, error) {
	result, err := s.MiotRequest("action", H{
		"did":  did,
		"siid": iid[0],
		"aiid": iid[1],
		"in":   in,
	})
	if err != nil {
		return nil, err
	}
	output, ok := result.(H)
	if !ok {
		return nil, fmt.Errorf("unexpected action result: %v", result)
	}
	return output, nil
}

func (c *Client) Speak(did, text string) (H, error) {
	return c.MiotAction(did, []int{5, 1}, []any{text})
}

// func (c *Client) GetVolume(did string) (float64, error) {
// 	out, err := c.MiotGetProp(did, Iid{Siid: 2, Piid: 1})
// 	if err != nil {
// 		return 0, err
// 	}
// 	return out["value"].(float64), nil
// }

// func (c *Client) SetVolume(did string, volume float64) (any, error) {
// 	return c.MiotSetProp(did, Iid{Siid: 2, Piid: 1}, volume)
// }

// func (c *Client) Mute(did string, state bool) (any, error) {
// 	return c.MiotSetProp(did, Iid{Siid: 2, Piid: 2}, []any{"0"})
// }

// func (c *Client) MicrophoneMute(did string, state bool) (H, error) {
// 	return c.MiotAction(did, []int{3, 1}, []any{state})
// }

// // https://miot-spec.org/miot-spec-v2/instance?type=urn:miot-spec-v2:device:speaker:0000A015:xiaomi-lx01:1
// func (c *Client) GetPlayState(did string) (float64, error) {
// 	out, err := c.MiotGetProp(did, Iid{Siid: 4, Piid: 1})
// 	if err != nil {
// 		return 0, err
// 	}
// 	return out["value"].(float64), nil
// }

// func (c *Client) Pause(did string) (H, error) {
// 	return c.MiotAction(did, []int{4, 1}, []any{})
// }

// func (c *Client) Play(did string) (H, error) {
// 	return c.MiotAction(did, []int{4, 2}, []any{})
// }

// func (c *Client) Next(did string) (H, error) {
// 	return c.MiotAction(did, []int{4, 3}, []any{})
// }

// func (c *Client) Previous(did string) (H, error) {
// 	return c.MiotAction(did, []int{4, 4}, []any{})
// }

// func (c *Client) WakeUp(did string) (H, error) {
// 	return c.MiotAction(did, []int{5, 2}, []any{})
// }

// func (c *Client) PlayRadio(did string) (H, error) {
// 	return c.MiotAction(did, []int{5, 3}, []any{})
// }

// func (c *Client) PlayMusic(did string) (H, error) {
// 	return c.MiotAction(did, []int{5, 4}, []any{})
// }

// func (c *Client) StopAlarm(did string) (H, error) {
// 	return c.MiotAction(did, []int{6, 1}, []any{})
// }

// func (c *Client) ExecuteTextDirective(did string, text string, slient bool) (H, error) {
// 	s := 1
// 	if slient {
// 		s = 0
// 	}
// 	return c.MiotAction(did, []int{5, 5}, []any{text, s})
// }

// func (c *Client) Say(did string, text string) (H, error) {
// 	return c.ExecuteTextDirective(did, text, false)
// }
