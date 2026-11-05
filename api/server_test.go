package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lsongdev/mihome-go/mihome"
)

// fake embeds Service so a test only implements what it needs; anything else
// panics.
type fake struct {
	Service
	err       error
	called    []string
	iid       mihome.Iid
	value     any
	in        []any
	result    *fakeResult
	languages []string
	specs     int
	homes     int
}

type fakeResult struct {
	prop   any
	action mihome.H
}

func (f *fake) HomeList() ([]mihome.Home, error) {
	f.homes++
	return []mihome.Home{{ID: "1", Name: "mine"}, {ID: "2", Name: "theirs", Shared: true}}, f.err
}

func (f *fake) HomeDevices(home mihome.Home) ([]mihome.DeviceInfo, error) {
	if home.ID == "2" {
		return nil, nil
	}
	return []mihome.DeviceInfo{
		{Did: "d1", Name: "lamp", Model: "m", Token: "secret-token", IsOnline: true, SpecType: "urn:lamp"},
		{Did: "d2", Name: "band", Model: "n"},
	}, nil
}

func (f *fake) FetchSpec(urn, language string) (*mihome.Spec, error) {
	f.specs++
	f.languages = append(f.languages, language)
	return &mihome.Spec{URN: urn, Name: "lamp spec"}, f.err
}

func (f *fake) ListMinaDevices(int) ([]mihome.DeviceData, error) {
	return []mihome.DeviceData{{DeviceID: "s1", MiotDID: "9", Name: "box", Presence: "online", Mac: "secret-mac"}}, f.err
}

func (f *fake) MiotGetProps(_ string, iids []mihome.Iid) ([]any, error) {
	f.iid = iids[0]
	return []any{map[string]any{"value": 1}}, f.err
}

func (f *fake) MiotSetProp(_ string, iid mihome.Iid, value any) (any, error) {
	f.iid, f.value = iid, value
	if f.result != nil {
		return f.result.prop, f.err
	}
	return nil, f.err
}

func (f *fake) MiotAction(_ string, _ []int, in []any) (mihome.H, error) {
	f.in = in
	if f.result != nil {
		return f.result.action, f.err
	}
	return mihome.H{}, f.err
}

func (f *fake) GetConversationRecords(string, int) ([]mihome.ConversationRecord, error) {
	return nil, f.err
}

func (f *fake) control(name string) (mihome.H, error) {
	f.called = append(f.called, name)
	return mihome.H{}, f.err
}

func (f *fake) PlayerPause(string) (mihome.H, error)          { return f.control("pause") }
func (f *fake) PlayerSetVolume(string, int) (mihome.H, error) { return f.control("volume") }
func (f *fake) TextToSpeech(string, string) (mihome.H, error) { return f.control("tts") }

func call(s http.Handler, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w
}

func TestHomesIncludeDevicesWithoutSecrets(t *testing.T) {
	w := call(New(&fake{}, Options{}), "GET", "/homes", "")
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, `"online":true`) || !strings.Contains(body, `"shared":true`) {
		t.Fatalf("%d %s", w.Code, body)
	}
	if strings.Contains(body, "secret") || strings.Contains(body, "null") {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestSpeakersDoNotExposeSecrets(t *testing.T) {
	w := call(New(&fake{}, Options{}), "GET", "/speakers", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret") || !strings.Contains(w.Body.String(), `"id":"s1"`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestDeviceLookup(t *testing.T) {
	s := New(&fake{}, Options{})
	if w := call(s, "GET", "/devices/d1", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"home":"mine"`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := call(s, "GET", "/devices/nope", ""); w.Code != 404 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := call(s, "GET", "/speakers/nope", ""); w.Code != 404 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestProps(t *testing.T) {
	f := &fake{}
	s := New(f, Options{})
	if w := call(s, "GET", "/devices/d1/props?iid=2.1", ""); w.Code != 200 || f.iid != (mihome.Iid{Siid: 2, Piid: 1}) {
		t.Fatalf("%d %s %+v", w.Code, w.Body, f.iid)
	}
	for _, target := range []string{"/devices/d1/props", "/devices/d1/props?iid=2", "/devices/d1/props?iid=a.b"} {
		if w := call(s, "GET", target, ""); w.Code != 400 {
			t.Errorf("%s: %d", target, w.Code)
		}
	}
	if w := call(s, "PUT", "/devices/d1/props", `{"siid":2,"piid":1,"value":false}`); w.Code != 200 || f.value != false {
		t.Fatalf("%d %s %v", w.Code, w.Body, f.value)
	}
	if w := call(s, "PUT", "/devices/d1/props", `{"siid":2,"piid":1}`); w.Code != 400 {
		t.Fatalf("missing value: %d", w.Code)
	}
}

func TestDeviceRefusalsAreErrors(t *testing.T) {
	for result, code := range map[*fakeResult]int{
		{[]any{map[string]any{"code": float64(-704030013)}}, mihome.H{"code": float64(-1)}}: 502,
		{[]any{map[string]any{"code": float64(0)}}, mihome.H{"code": float64(1)}}:           200,
	} {
		s := New(&fake{result: result}, Options{})
		if w := call(s, "PUT", "/devices/d1/props", `{"siid":2,"piid":1,"value":true}`); w.Code != code {
			t.Errorf("props: got %d, want %d: %s", w.Code, code, w.Body)
		}
		if w := call(s, "POST", "/devices/d1/actions", `{"siid":2,"aiid":1}`); w.Code != code {
			t.Errorf("action: got %d, want %d: %s", w.Code, code, w.Body)
		}
	}
}

func TestActionDefaultsToNoArguments(t *testing.T) {
	f := &fake{}
	if w := call(New(f, Options{}), "POST", "/devices/d1/actions", `{"siid":5,"aiid":1}`); w.Code != 200 || f.in == nil {
		t.Fatalf("%d %s %v", w.Code, w.Body, f.in)
	}
}

func TestSpeakerControlsValidateInput(t *testing.T) {
	f := &fake{}
	s := New(f, Options{})
	cases := []struct {
		method, target, body string
		code                 int
	}{
		{"PUT", "/speakers/s1/volume", `{"volume":0}`, 200},
		{"PUT", "/speakers/s1/volume", `{"volume":101}`, 400},
		{"PUT", "/speakers/s1/volume", `{}`, 400},
		{"POST", "/speakers/s1/tts", `{"text":"hi"}`, 200},
		{"POST", "/speakers/s1/tts", `{"text":"  "}`, 400},
		{"POST", "/speakers/s1/player", `{"action":"pause"}`, 200},
		{"POST", "/speakers/s1/player", `{"action":"reboot"}`, 400},
		{"POST", "/speakers/s1/player", `{"action":"pause","x":1}`, 400},
		{"GET", "/speakers/s1/conversations?limit=0", "", 400},
		{"GET", "/speakers/s1/conversations", "", 200},
	}
	for _, c := range cases {
		if w := call(s, c.method, c.target, c.body); w.Code != c.code {
			t.Errorf("%s %s %s: got %d, want %d: %s", c.method, c.target, c.body, w.Code, c.code, w.Body)
		}
	}
	if strings.Join(f.called, ",") != "volume,tts,pause" {
		t.Fatalf("called %v", f.called)
	}
}

func TestConversationsAreNeverNull(t *testing.T) {
	if w := call(New(&fake{}, Options{}), "GET", "/speakers/s1/conversations", ""); strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("%s", w.Body)
	}
}

func TestUpstreamErrors(t *testing.T) {
	cases := []struct {
		err  error
		code int
		want string
	}{
		{errors.New("boom"), 502, `"error":"boom"`},
		{&mihome.VerificationError{URL: "https://verify.example"}, 502, `"code":"verification_required"`},
		{mihome.ErrDeviceNotFound, 404, "device not found"},
	}
	for _, c := range cases {
		w := call(New(&fake{err: c.err}, Options{}), "GET", "/speakers/s1/conversations", "")
		if w.Code != c.code || !strings.Contains(w.Body.String(), c.want) {
			t.Errorf("%v: %d %s", c.err, w.Code, w.Body)
		}
	}
}

// memoryCache is a Cache in a map; age sets how old its entries appear.
type memoryCache struct {
	values map[string][]byte
	age    time.Duration
}

func newMemoryCache() *memoryCache { return &memoryCache{values: map[string][]byte{}} }

func (c *memoryCache) Get(key string) ([]byte, time.Time, bool, error) {
	value, ok := c.values[key]
	return value, time.Now().Add(-c.age), ok, nil
}

func (c *memoryCache) Set(key string, value []byte) error {
	c.values[key] = value
	return nil
}

func TestHomesAreCachedUntilTheyExpire(t *testing.T) {
	f, cache := &fake{}, newMemoryCache()
	s := New(f, Options{Cache: cache, DevicesTTL: time.Minute})
	first := call(s, "GET", "/homes", "")
	second := call(s, "GET", "/homes", "")
	if f.homes != 1 || first.Body.String() != second.Body.String() {
		t.Fatalf("upstream calls = %d\n%s\n%s", f.homes, first.Body, second.Body)
	}
	if strings.Contains(string(cache.values["homes"]), "secret") {
		t.Fatalf("secrets were cached: %s", cache.values["homes"])
	}
	// The device page shares the entry.
	if w := call(s, "GET", "/devices/d1", ""); w.Code != 200 || f.homes != 1 {
		t.Fatalf("%d, upstream calls = %d", w.Code, f.homes)
	}
	call(s, "GET", "/homes?refresh=1", "")
	if f.homes != 2 {
		t.Fatalf("refresh: upstream calls = %d", f.homes)
	}
	cache.age = 2 * time.Minute
	call(s, "GET", "/homes", "")
	if f.homes != 3 {
		t.Fatalf("expired: upstream calls = %d", f.homes)
	}
}

func TestExpiredHomesAreNotServedWhenUpstreamFails(t *testing.T) {
	f, cache := &fake{}, newMemoryCache()
	s := New(f, Options{Cache: cache, DevicesTTL: time.Minute})
	call(s, "GET", "/homes", "")
	cache.age, f.err = 2*time.Minute, &mihome.VerificationError{URL: "https://verify.example"}
	if w := call(s, "GET", "/homes", ""); w.Code != 502 || !strings.Contains(w.Body.String(), "verification_required") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestSpecIsCached(t *testing.T) {
	f, cache := &fake{}, newMemoryCache()
	s := New(f, Options{Cache: cache, DevicesTTL: time.Hour, SpecTTL: time.Hour})
	for i := 0; i < 2; i++ {
		if w := call(s, "GET", "/devices/d1/spec", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "lamp spec") {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
	if f.specs != 1 {
		t.Fatalf("spec fetched %d times", f.specs)
	}
	// An expired specification is still better than none.
	delete(cache.values, "homes")
	cache.age = 2 * time.Hour
	s = New(&specFailure{fake: f}, Options{Cache: cache, DevicesTTL: time.Hour, SpecTTL: time.Hour})
	if w := call(s, "GET", "/devices/d1/spec", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "lamp spec") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestSpecCacheSeparatesLanguages(t *testing.T) {
	f, cache := &fake{}, newMemoryCache()
	for _, language := range []string{"", "en", "zh_cn", "en"} {
		s := New(f, Options{Cache: cache, DevicesTTL: time.Hour, SpecTTL: time.Hour, SpecLanguage: language})
		if w := call(s, "GET", "/devices/d1/spec", ""); w.Code != 200 {
			t.Fatalf("%s: %d %s", language, w.Code, w.Body)
		}
	}
	if f.specs != 2 || strings.Join(f.languages, ",") != "zh_cn,en" {
		t.Fatalf("fetches = %d, languages = %v", f.specs, f.languages)
	}
}

type specFailure struct{ *fake }

func (f *specFailure) FetchSpec(string, string) (*mihome.Spec, error) { return nil, errors.New("down") }

func TestSpecOfDeviceWithoutOne(t *testing.T) {
	f := &fake{}
	w := call(New(f, Options{}), "GET", "/devices/d2/spec", "")
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "null" || f.specs != 0 {
		t.Fatalf("%d %s, fetched %d", w.Code, w.Body, f.specs)
	}
	if w := call(New(f, Options{}), "GET", "/devices/nope/spec", ""); w.Code != 404 {
		t.Fatalf("missing device: %d", w.Code)
	}
}

func TestSpecFailureWithoutCache(t *testing.T) {
	w := call(New(&specFailure{fake: &fake{}}, Options{}), "GET", "/devices/d1/spec", "")
	if w.Code != 502 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

// brokenCache fails every operation: the API must keep working without it.
type brokenCache struct{}

func (brokenCache) Get(string) ([]byte, time.Time, bool, error) {
	return nil, time.Time{}, false, errors.New("broken")
}
func (brokenCache) Set(string, []byte) error { return errors.New("broken") }

func TestBrokenCacheIsBypassed(t *testing.T) {
	if w := call(New(&fake{}, Options{Cache: brokenCache{}, DevicesTTL: time.Minute}), "GET", "/homes", ""); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestCorruptCacheEntryIsReloaded(t *testing.T) {
	f, cache := &fake{}, newMemoryCache()
	cache.values["homes"] = []byte("{not json")
	if w := call(New(f, Options{Cache: cache, DevicesTTL: time.Minute}), "GET", "/homes", ""); w.Code != 200 || f.homes != 1 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
