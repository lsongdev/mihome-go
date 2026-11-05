package mihome

import (
	"context"
	"encoding/json"
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
   {"iid":2,"type":"urn:miot-spec-v2:action:delay:00002812:dmaker-p45:1","description":"Delay","in":[3],"out":[1]}],
  "events":[
   {"iid":1007,"type":"urn:miot-spec-v2:event:fault:00005001:dmaker-p45:1","description":"Fault","arguments":[1,3]},
   {"iid":1008,"type":"urn:miot-spec-v2:event:click:00005002:dmaker-p45:1","description":"Click"}]}]}`

const fanLanguage = `{"data":{"zh_cn":{"service:002":"风扇","service:002:property:001":"开关",
 "service:002:property:002:valuelist:000":"一档","service:002:action:001":"切换","service:002:event:1007":"故障"}}}`

func TestFetchOriginalSpecDocuments(t *testing.T) {
	urn := "urn:miot-spec-v2:device:fan:0000A005:dmaker-p45:1"
	ctx := context.WithValue(context.Background(), struct{}{}, "request")
	client := newTestClient(t, "", "", "", func(req *http.Request) *http.Response {
		if req.Context() != ctx {
			t.Error("request context was lost")
		}
		if req.URL.Host != "miot-spec.org" {
			t.Fatalf("host = %s", req.URL.Host)
		}
		if strings.Contains(req.URL.Path, "multiLanguage") {
			if req.URL.Query().Get("urn") != urn {
				t.Fatal("translation URN missing")
			}
			return response(200, fanLanguage, "")
		}
		if req.URL.Query().Get("type") != urn {
			t.Fatal("spec URN missing")
		}
		return response(200, fanSpec, "")
	})
	raw, err := client.FetchSpec(ctx, urn)
	if err != nil || string(raw) != fanSpec {
		t.Fatalf("spec changed: %s, %v", raw, err)
	}
	translations, err := client.FetchSpecTranslations(ctx, urn)
	if err != nil || string(translations) != fanLanguage {
		t.Fatalf("translations changed: %s, %v", translations, err)
	}
}

func TestSpecRejectsInvalidDocuments(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `{"code":1}`, `{"type":"fan"}`, `not json`} {
		client := newTestClient(t, "", "", "", func(*http.Request) *http.Response { return response(200, body, "") })
		if _, err := client.FetchSpec(context.Background(), "urn"); err == nil {
			t.Errorf("spec accepted %s", body)
		}
		if _, err := client.FetchSpecTranslations(context.Background(), "urn"); err == nil {
			t.Errorf("translations accepted %s", body)
		}
	}
	client := newTestClient(t, "", "", "", func(*http.Request) *http.Response { return response(404, "missing", "") })
	if _, err := client.FetchSpec(context.Background(), "urn"); err == nil {
		t.Fatal("expected HTTP failure")
	}
	if _, err := client.FetchSpecTranslations(context.Background(), "urn"); err == nil {
		t.Fatal("expected translation HTTP failure")
	}
}

func TestOriginalDocumentsKeepUnknownFieldsAndAllLanguages(t *testing.T) {
	spec := `{"type":"urn:fan","services":[],"vendor":{"unknown":true}}`
	translations := `{"data":{"zh_cn":{"service:002":"风扇"},"ja":{"service:002":"ファン"}},"vendor":123}`
	client := newTestClient(t, "", "", "", func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Path, "multiLanguage") {
			return response(200, translations, "")
		}
		return response(200, spec, "")
	})
	raw, err := client.FetchSpec(context.Background(), "urn:fan")
	if err != nil || string(raw) != spec {
		t.Fatalf("spec = %s, %v", raw, err)
	}
	raw, err = client.FetchSpecTranslations(context.Background(), "urn:fan")
	if err != nil || string(raw) != translations {
		t.Fatalf("translations = %s, %v", raw, err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
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
