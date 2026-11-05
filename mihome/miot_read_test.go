package mihome

import (
	"encoding/json"
	"testing"
)

func TestMiotGetPropsRecoversRejectedBatchEntriesByID(t *testing.T) {
	var requests [][]any
	client := newMiioClient(t, func(path string, data H) string {
		params := data["params"].([]any)
		requests = append(requests, params)
		if len(params) > 1 {
			// Upstream reorders results. Only the generic failures should be retried.
			return `{"code":0,"result":[{"siid":2,"piid":1,"code":-704002000},{"siid":1,"piid":1,"code":0,"value":"Xiaomi"},{"siid":4,"piid":1,"code":-704030013},{"siid":5,"piid":3,"code":-704002000}]}`
		}
		property := params[0].(map[string]any)
		if property["siid"] == float64(2) {
			if property["piid"] != float64(1) || property["did"] != "speaker" {
				t.Fatalf("wrong retry: %v", property)
			}
			return `{"code":0,"result":[{"siid":2,"piid":1,"code":0,"value":50}]}`
		}
		// A real unsupported property stays an error, with no further retry.
		return `{"code":0,"result":[{"siid":5,"piid":3,"code":-704002000}]}`
	})
	results, err := client.MiotGetProps("speaker", []Iid{{1, 1}, {2, 1}, {4, 1}, {5, 3}})
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 || len(results) != 4 {
		t.Fatalf("requests=%d, results=%v", len(requests), results)
	}
	if results[0].(H)["value"] != float64(50) || results[1].(H)["value"] != "Xiaomi" || results[2].(H)["code"] != float64(-704030013) || results[3].(H)["code"] != float64(-704002000) {
		t.Fatalf("results=%v", results)
	}
}

func TestMiotGetPropsPreservesBatchResultWhenRetryFails(t *testing.T) {
	for _, retry := range []string{`{"code":-1,"message":"temporary failure"}`, `{"code":0,"result":[]}`} {
		calls := 0
		client := newMiioClient(t, func(path string, data H) string {
			calls++
			if calls == 1 {
				return `{"code":0,"result":[{"siid":2,"piid":1,"code":-704002000},{"siid":1,"piid":1,"code":0,"value":"Xiaomi"}]}`
			}
			return retry
		})
		results, err := client.MiotGetProps("speaker", []Iid{{2, 1}, {1, 1}})
		if err != nil || len(results) != 2 || calls != 2 {
			t.Fatalf("results=%v err=%v calls=%d", results, err, calls)
		}
		if results[0].(H)["code"] != float64(-704002000) || results[1].(H)["value"] != "Xiaomi" {
			t.Fatalf("lost original results: %v", results)
		}
	}
}

func TestMiotGetPropsDoesNotRetrySingleRead(t *testing.T) {
	calls := 0
	client := newMiioClient(t, func(path string, data H) string {
		calls++
		return `{"code":0,"result":[{"siid":2,"piid":1,"code":-704002000}]}`
	})
	results, err := client.MiotGetProps("speaker", []Iid{{2, 1}})
	if err != nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	raw, _ := json.Marshal(results)
	if string(raw) == "null" {
		t.Fatal("lost error result")
	}
}
