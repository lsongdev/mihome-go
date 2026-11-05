package mihome

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func newMiioClient(t *testing.T, f func(path string, data H) string) *Client {
	t.Helper()
	client := newTestClient(t, "", "user", "password", func(req *http.Request) *http.Response {
		if err := req.ParseForm(); err != nil {
			t.Fatal(err)
		}
		data := H{}
		if err := json.Unmarshal([]byte(req.PostForm.Get("data")), &data); err != nil {
			t.Fatal(err)
		}
		path := strings.TrimPrefix(req.URL.Path, "/app")
		return response(200, f(path, data), "")
	})
	client.Token = NewTokens()
	client.Token.Sids[MIIO_SID] = SidToken{Ssecurity: "c2VjdXJpdHk=", ServiceToken: "t"}
	return client
}

func TestHomeListIncludesSharedHomes(t *testing.T) {
	client := newMiioClient(t, func(path string, data H) string {
		if data["fetch_share"] != true {
			t.Errorf("shared homes were not requested: %v", data)
		}
		return `{"code":0,"result":{
			"homelist":[{"id":"872001099423","name":"mine","uid":1}],
			"share_home_list":[{"id":"384001595038","name":"theirs","uid":2885686517}]
		}}`
	})
	homes, err := client.HomeList()
	if err != nil {
		t.Fatal(err)
	}
	if len(homes) != 2 || homes[0].Shared || !homes[1].Shared {
		t.Fatalf("unexpected homes: %+v", homes)
	}
	if homes[1].ID != "384001595038" || homes[1].Owner != "2885686517" {
		t.Fatalf("ids were altered: %+v", homes[1])
	}
}

func TestHomeDevicesFollowsPagination(t *testing.T) {
	var starts []any
	client := newMiioClient(t, func(path string, data H) string {
		starts = append(starts, data["start_did"])
		if data["home_id"] != float64(384001595038) || data["home_owner"] != float64(2885686517) {
			t.Errorf("wrong home: %v", data)
		}
		if data["start_did"] == "" {
			return `{"code":0,"result":{"device_info":[{"did":"1","name":"a"}],"has_more":true,"max_did":"1"}}`
		}
		return `{"code":0,"result":{"device_info":[{"did":"2","name":"b"}],"has_more":false,"max_did":""}}`
	})
	devices, err := client.HomeDevices(Home{ID: "384001595038", Owner: "2885686517"})
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 || devices[1].Name != "b" || len(starts) != 2 || starts[1] != "1" {
		t.Fatalf("devices = %+v, starts = %v", devices, starts)
	}
}
