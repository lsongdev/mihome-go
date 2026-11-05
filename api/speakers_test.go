package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/lsongdev/mihome-go/mihome"
)

type availabilityFake struct {
	*fake
	online       bool
	presence     string
	did          string
	homeErr      error
	speakerCalls int
}

func (f *availabilityFake) HomeList() ([]mihome.Home, error) {
	if f.homeErr != nil {
		return nil, f.homeErr
	}
	return f.fake.HomeList()
}
func (f *availabilityFake) HomeDevices(mihome.Home) ([]mihome.DeviceInfo, error) {
	return []mihome.DeviceInfo{{Did: "touch-did", Name: "same name", IsOnline: f.online}}, nil
}
func (f *availabilityFake) ListMinaDevices(int) ([]mihome.DeviceData, error) {
	f.speakerCalls++
	return []mihome.DeviceData{{DeviceID: "mina-id", MiotDID: f.did, Name: "same name", Presence: f.presence}}, nil
}
func speakerStatus(t *testing.T, s *Server, path string) bool {
	t.Helper()
	w := call(s, "GET", path, "")
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if path == "/speakers" || path == "/speakers?refresh=1" {
		var list []Speaker
		if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list) != 1 {
			t.Fatalf("invalid list: %s", w.Body)
		}
		return list[0].Online
	}
	var device Speaker
	if err := json.Unmarshal(w.Body.Bytes(), &device); err != nil {
		t.Fatal(err)
	}
	return device.Online
}
func TestSpeakerAvailabilityUsesMiotDID(t *testing.T) {
	for _, tc := range []struct {
		name, did, presence string
		homeOnline, want    bool
	}{
		{"Mi Home online", "touch-did", "offline", true, true},
		{"Mi Home offline", "touch-did", "online", false, false},
		{"unmatched DID", "other", "online", false, true},
		{"missing DID", "", "offline", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &availabilityFake{fake: &fake{}, online: tc.homeOnline, presence: tc.presence, did: tc.did}
			s := New(f, Options{})
			for _, path := range []string{"/speakers", "/speakers/mina-id"} {
				if got := speakerStatus(t, s, path); got != tc.want {
					t.Fatalf("%s: online=%v, want %v", path, got, tc.want)
				}
			}
		})
	}
}
func TestCachedSpeakerAvailabilityFollowsHomesAndRefresh(t *testing.T) {
	f := &availabilityFake{fake: &fake{}, online: true, presence: "offline", did: "touch-did"}
	cache := newMemoryCache()
	s := New(f, Options{Cache: cache, DevicesTTL: time.Hour})
	if !speakerStatus(t, s, "/speakers") {
		t.Fatal("speaker should be online")
	}
	// Refresh the device list while the speaker inventory is still cached.
	f.online = false
	if w := call(s, "GET", "/homes?refresh=1", ""); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if speakerStatus(t, s, "/speakers") || f.speakerCalls != 1 {
		t.Fatal("cached speaker inventory must use the latest home status")
	}
	f.online = true
	if !speakerStatus(t, s, "/speakers?refresh=1") || f.speakerCalls != 2 {
		t.Fatal("speaker refresh must also refresh home availability")
	}
	// The cache must retain MiNA's original presence for the fallback case.
	f.homeErr = errors.New("home service unavailable")
	delete(cache.values, "homes")
	if speakerStatus(t, s, "/speakers") {
		t.Fatal("unavailable home service must fall back to original MiNA presence")
	}
}
