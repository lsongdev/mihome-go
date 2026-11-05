package mihome

import (
	"net/http"
	"strings"
	"testing"
)

const fanSpec = `{"type":"urn:miot-spec-v2:device:fan:0000A005:dmaker-p45:1","description":"Fan","services":[
 {"iid":2,"type":"urn:miot-spec-v2:service:fan:00007808:dmaker-p45:1","description":"Fan",
  "properties":[
   {"iid":1,"type":"urn:miot-spec-v2:property:on:00000006:dmaker-p45:1","description":"Switch Status","format":"bool","access":["read","write"],"unit":"none"},
   {"iid":2,"type":"urn:miot-spec-v2:property:fan-level:00000016:dmaker-p45:1","description":"Fan Level","format":"uint8","access":["read","write"],
    "value-list":[{"value":5,"description":"Level1"},{"value":9,"description":"Level2"}]},
   {"iid":3,"type":"urn:miot-spec-v2:property:brightness:0000000D:dmaker-p45:1","description":"Brightness","format":"uint8","access":["read"],"unit":"percentage","value-range":[1,100,5]}],
  "actions":[
   {"iid":1,"type":"urn:miot-spec-v2:action:toggle:00002811:dmaker-p45:1","description":"Toggle","in":[],"out":[]},
   {"iid":2,"type":"urn:miot-spec-v2:action:delay:00002812:dmaker-p45:1","description":"Delay","in":[3],"out":[]}]}]}`

const fanLanguage = `{"data":{"zh_cn":{"service:002":"风扇","service:002:property:001":"开关",
 "service:002:property:002:valuelist:000":"一档","service:002:action:001":"切换"}}}`

func TestFetchSpec(t *testing.T) {
	urn := "urn:miot-spec-v2:device:fan:0000A005:dmaker-p45:1"
	client := newTestClient(t, "", "", "", func(req *http.Request) *http.Response {
		if req.URL.Host != "miot-spec.org" {
			t.Errorf("unexpected host %s", req.URL.Host)
		}
		if strings.Contains(req.URL.Path, "multiLanguage") {
			if req.URL.Query().Get("urn") != urn {
				t.Errorf("urn = %q", req.URL.Query().Get("urn"))
			}
			return response(200, fanLanguage, "")
		}
		if req.URL.Query().Get("type") != urn {
			t.Errorf("type = %q", req.URL.Query().Get("type"))
		}
		return response(200, fanSpec, "")
	})
	spec, err := client.FetchSpec(urn)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Type != "fan" || len(spec.Services) != 1 {
		t.Fatalf("spec = %+v", spec)
	}
	service := spec.Services[0]
	if service.Name != "风扇" || service.Type != "fan" {
		t.Fatalf("service = %+v", service)
	}
	on, level, brightness := service.Properties[0], service.Properties[1], service.Properties[2]
	if on.Name != "开关" || on.Unit != "" || on.Type != "on" {
		t.Errorf("on = %+v", on)
	}
	// Translated by position: the first option has value 5.
	if level.Options[0].Name != "一档" || level.Options[0].Value != float64(5) || level.Options[1].Name != "Level2" {
		t.Errorf("level = %+v", level)
	}
	if brightness.Name != "Brightness" || *brightness.Range != (SpecRange{Min: 1, Max: 100, Step: 5}) || brightness.Unit != "percentage" {
		t.Errorf("brightness = %+v", brightness)
	}
	if service.Actions[0].Name != "切换" || service.Actions[1].In[0] != 3 || service.Actions[0].In == nil {
		t.Errorf("actions = %+v", service.Actions)
	}
}

func TestFetchSpecWithoutTranslations(t *testing.T) {
	client := newTestClient(t, "", "", "", func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Path, "multiLanguage") {
			return response(500, "", "")
		}
		return response(200, fanSpec, "")
	})
	spec, err := client.FetchSpec("urn:x:device:fan:1")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Services[0].Name != "Fan" {
		t.Fatalf("service = %+v", spec.Services[0])
	}
}

func TestFetchSpecFailure(t *testing.T) {
	client := newTestClient(t, "", "", "", func(req *http.Request) *http.Response {
		return response(404, "missing", "")
	})
	if _, err := client.FetchSpec("urn:x:device:fan:1"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestMiotGetPropsSplitsLargeRequests(t *testing.T) {
	var sizes []int
	client := newMiioClient(t, func(path string, data H) string {
		params := data["params"].([]any)
		sizes = append(sizes, len(params))
		var results []string
		for range params {
			results = append(results, `{"code":0,"value":1}`)
		}
		return `{"code":0,"result":[` + strings.Join(results, ",") + `]}`
	})
	iids := make([]Iid, 65)
	results, err := client.MiotGetProps("d", iids)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 65 || len(sizes) != 3 || sizes[0] != miotPropsLimit || sizes[2] != 5 {
		t.Fatalf("results = %d, sizes = %v", len(results), sizes)
	}
}
