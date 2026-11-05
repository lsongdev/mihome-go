package miservice

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const specBase = "https://miot-spec.org"

// Spec describes what a device can do, from the public MIoT specification.
// Names are in Chinese when a translation exists.
type Spec struct {
	URN      string        `json:"urn"`
	Type     string        `json:"type"`
	Name     string        `json:"name"`
	Services []SpecService `json:"services"`
}

type SpecService struct {
	Iid        int            `json:"iid"`
	Type       string         `json:"type"`
	Name       string         `json:"name"`
	Properties []SpecProperty `json:"properties"`
	Actions    []SpecAction   `json:"actions"`
}

type SpecProperty struct {
	Iid     int          `json:"iid"`
	Type    string       `json:"type"`
	Name    string       `json:"name"`
	Format  string       `json:"format"`
	Access  []string     `json:"access"`
	Unit    string       `json:"unit,omitempty"`
	Range   *SpecRange   `json:"range,omitempty"`
	Options []SpecOption `json:"options,omitempty"`
}

type SpecRange struct {
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	Step float64 `json:"step"`
}

type SpecOption struct {
	Value any    `json:"value"`
	Name  string `json:"name"`
}

type SpecAction struct {
	Iid  int    `json:"iid"`
	Type string `json:"type"`
	Name string `json:"name"`
	// In lists the iids of the service's properties that are the arguments.
	In []int `json:"in"`
}

type rawSpec struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Services    []struct {
		Iid         int    `json:"iid"`
		Type        string `json:"type"`
		Description string `json:"description"`
		Properties  []struct {
			Iid         int       `json:"iid"`
			Type        string    `json:"type"`
			Description string    `json:"description"`
			Format      string    `json:"format"`
			Access      []string  `json:"access"`
			Unit        string    `json:"unit"`
			ValueRange  []float64 `json:"value-range"`
			ValueList   []struct {
				Value       any    `json:"value"`
				Description string `json:"description"`
			} `json:"value-list"`
		} `json:"properties"`
		Actions []struct {
			Iid         int    `json:"iid"`
			Type        string `json:"type"`
			Description string `json:"description"`
			In          []int  `json:"in"`
		} `json:"actions"`
	} `json:"services"`
}

// FetchSpec downloads the specification named by a device's SpecType. Missing
// translations only leave the English names in place.
func (c *Client) FetchSpec(urn string) (*Spec, error) {
	var raw rawSpec
	err := c.getSpecJSON("/miot-spec-v2/instance?type="+url.QueryEscape(urn), &raw)
	if err != nil {
		return nil, fmt.Errorf("获取设备规格失败: %w", err)
	}
	var language struct {
		Data struct {
			Chinese map[string]string `json:"zh_cn"`
		} `json:"data"`
	}
	if err := c.getSpecJSON("/instance/v2/multiLanguage?urn="+url.QueryEscape(urn), &language); err != nil {
		language.Data.Chinese = nil
	}
	return newSpec(urn, raw, language.Data.Chinese), nil
}

func (c *Client) getSpecJSON(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, specBase+path, nil)
	if err != nil {
		return err
	}
	res, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		io.Copy(io.Discard, res.Body)
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func newSpec(urn string, raw rawSpec, names map[string]string) *Spec {
	name := func(fallback, format string, args ...any) string {
		if translated := names[fmt.Sprintf(format, args...)]; translated != "" {
			return translated
		}
		return fallback
	}
	spec := &Spec{URN: urn, Type: urnName(urn), Name: raw.Description, Services: []SpecService{}}
	for _, s := range raw.Services {
		service := SpecService{
			Iid:        s.Iid,
			Type:       urnName(s.Type),
			Name:       name(s.Description, "service:%03d", s.Iid),
			Properties: []SpecProperty{},
			Actions:    []SpecAction{},
		}
		for _, p := range s.Properties {
			property := SpecProperty{
				Iid:    p.Iid,
				Type:   urnName(p.Type),
				Name:   name(p.Description, "service:%03d:property:%03d", s.Iid, p.Iid),
				Format: p.Format,
				Access: p.Access,
			}
			if p.Unit != "none" {
				property.Unit = p.Unit
			}
			if len(p.ValueRange) >= 2 {
				property.Range = &SpecRange{Min: p.ValueRange[0], Max: p.ValueRange[1], Step: 1}
				if len(p.ValueRange) >= 3 && p.ValueRange[2] > 0 {
					property.Range.Step = p.ValueRange[2]
				}
			}
			// Translations of a value list are keyed by position, not value.
			for i, option := range p.ValueList {
				property.Options = append(property.Options, SpecOption{
					Value: option.Value,
					Name:  name(option.Description, "service:%03d:property:%03d:valuelist:%03d", s.Iid, p.Iid, i),
				})
			}
			service.Properties = append(service.Properties, property)
		}
		for _, a := range s.Actions {
			in := a.In
			if in == nil {
				in = []int{}
			}
			service.Actions = append(service.Actions, SpecAction{
				Iid:  a.Iid,
				Type: urnName(a.Type),
				Name: name(a.Description, "service:%03d:action:%03d", s.Iid, a.Iid),
				In:   in,
			})
		}
		spec.Services = append(spec.Services, service)
	}
	return spec
}

// urnName returns "fan" for "urn:miot-spec-v2:service:fan:00007808:dmaker-p45:1".
func urnName(urn string) string {
	parts := strings.Split(urn, ":")
	if len(parts) < 4 {
		return ""
	}
	return parts[3]
}
