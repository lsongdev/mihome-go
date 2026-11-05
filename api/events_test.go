package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lsongdev/mihome-go/mihome"
)

type fakeStream struct {
	events chan mihome.Event
	mu     sync.Mutex
	dids   []string
	err    error
	closed bool
}

func newFakeStream() *fakeStream { return &fakeStream{events: make(chan mihome.Event, 8)} }

func (f *fakeStream) Events() <-chan mihome.Event { return f.events }
func (f *fakeStream) Connected() bool             { return true }
func (f *fakeStream) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}
func (f *fakeStream) SetDevices(_ context.Context, dids []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dids = dids
	return f.err
}
func (f *fakeStream) devices() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dids
}

type eventsFake struct {
	fake
	ready    bool
	code     string
	readyMu  sync.Mutex
	failURL  error
	failCode error
}

func (f *eventsFake) HomeList() ([]mihome.Home, error) {
	return []mihome.Home{
		{ID: "1", Name: "mine", DeviceIDs: []string{"d1", "lumi.child"}},
		{ID: "2", Name: "theirs", Shared: true, DeviceIDs: []string{"shared"}},
	}, nil
}
func (f *eventsFake) OAuthReady() bool {
	f.readyMu.Lock()
	defer f.readyMu.Unlock()
	return f.ready
}
func (f *eventsFake) OAuthURL() (string, error) { return "https://login.example/", f.failURL }
func (f *eventsFake) CompleteOAuth(address string) error {
	f.readyMu.Lock()
	defer f.readyMu.Unlock()
	if f.failCode != nil {
		return f.failCode
	}
	f.code, f.ready = address, true
	return nil
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func runEvents(t *testing.T, s *Server) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.RunEvents(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
}

func sseReader(t *testing.T, server *httptest.Server, path string) func() Event {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+path, nil)
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	lines := make(chan string, 16)
	go func() {
		scanner := bufio.NewScanner(res.Body)
		for scanner.Scan() {
			if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
				lines <- data
			}
		}
	}()
	return func() Event {
		t.Helper()
		select {
		case line := <-lines:
			var event Event
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				t.Fatal(err)
			}
			return event
		case <-time.After(5 * time.Second):
			t.Fatal("no event")
			return Event{}
		}
	}
}

func TestEventsFlowFromStreamToClients(t *testing.T) {
	stream := newFakeStream()
	f := &eventsFake{ready: true}
	s := New(f, Options{OpenEvents: func(context.Context, mihome.EventOptions) (EventStream, error) { return stream, nil }})
	server := httptest.NewServer(s)
	// Cleanups run last first: readers end before Close, which waits for them.
	t.Cleanup(server.Close)
	runEvents(t, s)
	waitFor(t, func() bool { return len(stream.devices()) > 0 })
	if got := strings.Join(stream.devices(), ","); got != "d1,lumi.child,d2" {
		t.Fatalf("devices = %s", got)
	}
	next := sseReader(t, server, "/events")
	// The reader only gets new events, so wait until it is attached.
	waitFor(t, func() bool {
		s.hub.mu.Lock()
		defer s.hub.mu.Unlock()
		return len(s.hub.subscribers) == 1
	})
	at := time.Now()
	stream.events <- mihome.Event{Type: mihome.EventProperty, DeviceID: "d1", Siid: 2, Piid: 1, Value: true, Time: at}
	stream.events <- mihome.Event{Type: mihome.EventOccurred, DeviceID: "lumi.child", Siid: 3, Eiid: 1, Arguments: []any{}}
	stream.events <- mihome.Event{Type: mihome.EventAvailability, DeviceID: "d1", Online: false}

	event := next()
	if event.Type != EventPropertyChanged || event.DeviceID != "d1" || event.Data["value"] != true || event.Seq == 0 || !event.Time.Equal(at) {
		t.Fatalf("%+v", event)
	}
	if event := next(); event.Type != EventDeviceEvent || event.Data["eiid"] != float64(1) {
		t.Fatalf("%+v", event)
	}
	if event := next(); event.Type != EventAvailabilityChanged || event.Data["online"] != false {
		t.Fatalf("%+v", event)
	}

	// A client that reconnects gets what it missed.
	resumed := sseReader(t, server, fmt.Sprintf("/events?after=%d", event.Seq-1))
	if got := resumed(); got.Seq != event.Seq {
		t.Fatalf("resumed at %d, want %d", got.Seq, event.Seq)
	}
	status := call(s, "GET", "/events/status", "").Body.String()
	if !strings.Contains(status, `"connected":true`) || !strings.Contains(status, `"devices":3`) {
		t.Fatalf("status = %s", status)
	}
}

func TestSharedHomesAreFollowedOnlyWhenAsked(t *testing.T) {
	stream := newFakeStream()
	s := New(&eventsFake{ready: true}, Options{
		EventsIncludeShared: true,
		OpenEvents:          func(context.Context, mihome.EventOptions) (EventStream, error) { return stream, nil },
	})
	runEvents(t, s)
	waitFor(t, func() bool { return strings.Contains(strings.Join(stream.devices(), ","), "shared") })
}

func TestLoginStartsTheStream(t *testing.T) {
	stream := newFakeStream()
	f := &eventsFake{}
	opened := make(chan struct{}, 1)
	s := New(f, Options{OpenEvents: func(context.Context, mihome.EventOptions) (EventStream, error) {
		opened <- struct{}{}
		return stream, nil
	}})
	runEvents(t, s)
	if body := call(s, "GET", "/events/status", "").Body.String(); !strings.Contains(body, `"enabled":true`) || !strings.Contains(body, `"authorized":false`) {
		t.Fatalf("status = %s", body)
	}
	if w := call(s, "GET", "/oauth", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "login.example") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	select {
	case <-opened:
		t.Fatal("opened before login")
	case <-time.After(50 * time.Millisecond):
	}
	if w := call(s, "POST", "/oauth", `{"url":"http://x/?code=1&state=2"}`); w.Code != 200 || f.code == "" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	select {
	case <-opened:
	case <-time.After(5 * time.Second):
		t.Fatal("login did not start the stream")
	}
}

func TestLoginErrors(t *testing.T) {
	f := &eventsFake{failURL: errors.New("no client"), failCode: errors.New("bad code")}
	s := New(f, Options{})
	if w := call(s, "GET", "/oauth", ""); w.Code != 400 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := call(s, "POST", "/oauth", `{"url":"x"}`); w.Code != 400 || !strings.Contains(w.Body.String(), "bad code") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestEventsDisabledWithoutConfiguration(t *testing.T) {
	s := New(&eventsFake{ready: true}, Options{})
	s.RunEvents(context.Background()) // returns at once
	if body := call(s, "GET", "/events/status", "").Body.String(); !strings.Contains(body, `"enabled":false`) {
		t.Fatalf("status = %s", body)
	}
}

func TestStreamFailureIsReportedAndRetried(t *testing.T) {
	f := &eventsFake{ready: true}
	s := New(f, Options{OpenEvents: func(context.Context, mihome.EventOptions) (EventStream, error) {
		return nil, errors.New("broker down")
	}})
	runEvents(t, s)
	waitFor(t, func() bool {
		return strings.Contains(call(s, "GET", "/events/status", "").Body.String(), "broker down")
	})
}

func TestRefusedDevicesAreReported(t *testing.T) {
	stream := newFakeStream()
	stream.err = errors.New("broker 拒绝订阅 1 台设备")
	s := New(&eventsFake{ready: true}, Options{OpenEvents: func(context.Context, mihome.EventOptions) (EventStream, error) { return stream, nil }})
	runEvents(t, s)
	waitFor(t, func() bool {
		return strings.Contains(call(s, "GET", "/events/status", "").Body.String(), "拒绝订阅")
	})
}

func TestReconnectIsVisibleToClients(t *testing.T) {
	stream := newFakeStream()
	var hooks mihome.EventOptions
	opened := make(chan struct{})
	s := New(&eventsFake{ready: true}, Options{OpenEvents: func(_ context.Context, options mihome.EventOptions) (EventStream, error) {
		hooks = options
		close(opened)
		return stream, nil
	}})
	runEvents(t, s)
	<-opened
	waitFor(t, func() bool { return len(stream.devices()) > 0 })
	hooks.OnDisconnect()
	hooks.OnConnect()
	backlog, _, cancel := s.hub.subscribe(0)
	defer cancel()
	var kinds []string
	for _, event := range backlog {
		kinds = append(kinds, event.Type)
	}
	if strings.Join(kinds, ",") != "stream.connected,stream.disconnected,stream.connected" {
		t.Fatalf("events = %v", kinds)
	}
}

func TestHubKeepsRecentHistoryAndDropsSlowClients(t *testing.T) {
	h := newEventHub()
	_, slow, cancel := h.subscribe(^uint64(0))
	defer cancel()
	for i := 0; i < eventHistory+50; i++ {
		h.publish(Event{Type: "x"})
	}
	backlog, _, cancelAll := h.subscribe(0)
	defer cancelAll()
	if len(backlog) != eventHistory || backlog[0].Seq != 51 {
		t.Fatalf("backlog = %d from %d", len(backlog), backlog[0].Seq)
	}
	drained := 0
	for range slow {
		drained++
	}
	if drained != subscriberBuffer {
		t.Fatalf("slow client got %d", drained)
	}
}

func TestEventsRejectInvalidCursor(t *testing.T) {
	req := httptest.NewRequest("GET", "/events?after=x", nil)
	w := httptest.NewRecorder()
	New(&eventsFake{}, Options{}).ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("%d", w.Code)
	}
}
