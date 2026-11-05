package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/lsongdev/mihome-go/mihome"
)

// Event types, named as in zigbee-go so one consumer can read both.
const (
	EventPropertyChanged     = "device.property_changed"
	EventDeviceEvent         = "device.event"
	EventAvailabilityChanged = "device.availability_changed"
	// EventStreamConnected and EventStreamDisconnected mark a reconnect: changes
	// during the outage were not delivered, so clients should read state again.
	EventStreamConnected    = "stream.connected"
	EventStreamDisconnected = "stream.disconnected"
)

const (
	eventHistory    = 1024
	deviceRefresh   = 10 * time.Minute
	watchRetryDelay = 30 * time.Second
	// A client that cannot keep up is dropped and resumes from its last event.
	subscriberBuffer = 256
)

// Event is one entry of the event stream.
type Event struct {
	Seq      uint64         `json:"seq"`
	Time     time.Time      `json:"time"`
	Type     string         `json:"type"`
	DeviceID string         `json:"device_id,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

// EventStream is what the API needs of a mihome.EventStream.
type EventStream interface {
	Events() <-chan mihome.Event
	SetDevices(ctx context.Context, dids []string) error
	Connected() bool
	Close() error
}

// eventHub keeps recent events in memory and passes new ones to subscribers.
type eventHub struct {
	mu          sync.Mutex
	seq         uint64
	history     []Event
	subscribers map[chan Event]struct{}
	devices     int
	lastError   string
	stream      EventStream
}

func newEventHub() *eventHub {
	return &eventHub{subscribers: make(map[chan Event]struct{})}
}

func (h *eventHub) publish(event Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	event.Seq = h.seq
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	h.history = append(h.history, event)
	if len(h.history) > eventHistory {
		h.history = append(h.history[:0], h.history[len(h.history)-eventHistory:]...)
	}
	for subscriber := range h.subscribers {
		select {
		case subscriber <- event:
		default:
			delete(h.subscribers, subscriber)
			close(subscriber)
		}
	}
}

// subscribe returns the events after a sequence number and a channel of later
// ones, which is closed if the subscriber falls behind.
func (h *eventHub) subscribe(after uint64) ([]Event, chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var backlog []Event
	for _, event := range h.history {
		if event.Seq > after {
			backlog = append(backlog, event)
		}
	}
	subscriber := make(chan Event, subscriberBuffer)
	h.subscribers[subscriber] = struct{}{}
	return backlog, subscriber, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subscribers[subscriber]; ok {
			delete(h.subscribers, subscriber)
			close(subscriber)
		}
	}
}

func (h *eventHub) setError(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastError = ""
	if err != nil {
		h.lastError = err.Error()
	}
}

func hubEvent(event mihome.Event) Event {
	out := Event{Time: event.Time, DeviceID: event.DeviceID}
	switch event.Type {
	case mihome.EventProperty:
		out.Type = EventPropertyChanged
		out.Data = map[string]any{"siid": event.Siid, "piid": event.Piid, "value": event.Value}
	case mihome.EventOccurred:
		out.Type = EventDeviceEvent
		out.Data = map[string]any{"siid": event.Siid, "eiid": event.Eiid, "arguments": event.Arguments}
	case mihome.EventAvailability:
		out.Type = EventAvailabilityChanged
		out.Data = map[string]any{"online": event.Online}
	}
	return out
}

// RunEvents keeps the realtime stream going until ctx ends. It does nothing without
// Options.OpenEvents, and waits for the login when that is missing.
func (s *Server) RunEvents(ctx context.Context) {
	if s.options.OpenEvents == nil {
		return
	}
	for ctx.Err() == nil {
		if s.service.OAuthReady() {
			err := s.watch(ctx)
			if ctx.Err() != nil {
				return
			}
			s.hub.setError(err)
			log.Printf("events: %v", err)
		}
		select {
		case <-ctx.Done():
		case <-s.authorized:
		case <-time.After(watchRetryDelay):
		}
	}
}

func (s *Server) watch(ctx context.Context) error {
	connect, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	stream, err := s.options.OpenEvents(connect, mihome.EventOptions{
		OnConnect: func() {
			s.hub.setError(nil)
			s.hub.publish(Event{Type: EventStreamConnected})
		},
		OnDisconnect: func() { s.hub.publish(Event{Type: EventStreamDisconnected}) },
		OnError: func(err error) {
			s.hub.setError(err)
			log.Printf("events: %v", err)
		},
	})
	if err != nil {
		return err
	}
	defer stream.Close()
	s.setStream(stream)
	defer s.setStream(nil)
	s.hub.setError(nil)
	s.hub.publish(Event{Type: EventStreamConnected})
	s.followDevices(ctx, stream)

	refresh := time.NewTicker(deviceRefresh)
	defer refresh.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-refresh.C:
			s.followDevices(ctx, stream)
		case event, open := <-stream.Events():
			if !open {
				return errors.New("event stream closed")
			}
			s.hub.publish(hubEvent(event))
		}
	}
}

func (s *Server) setStream(stream EventStream) {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	s.hub.stream = stream
}

// followDevices subscribes to the devices of the homes. A failure keeps the current
// set: the next refresh tries again.
func (s *Server) followDevices(ctx context.Context, stream EventStream) {
	dids, err := s.eventDevices()
	if err != nil {
		s.hub.setError(err)
		log.Printf("events: list devices: %v", err)
		return
	}
	timeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	err = stream.SetDevices(timeout, dids)
	s.hub.setError(err)
	if err != nil {
		log.Printf("events: %v", err)
	}
	s.hub.mu.Lock()
	s.hub.devices = len(dids)
	s.hub.mu.Unlock()
}

func (s *Server) eventDevices() ([]string, error) {
	homes, err := s.service.HomeList()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var dids []string
	add := func(did string) {
		if did != "" && !seen[did] {
			seen[did] = true
			dids = append(dids, did)
		}
	}
	for _, home := range homes {
		if home.Shared && !s.options.EventsIncludeShared {
			continue
		}
		for _, did := range home.DeviceIDs {
			add(did)
		}
		devices, err := s.service.HomeDevices(home)
		if err != nil {
			return nil, err
		}
		for _, device := range devices {
			add(device.Did)
		}
	}
	return dids, nil
}

// EventStatus is the state of the realtime stream.
type EventStatus struct {
	Enabled    bool   `json:"enabled"`
	Authorized bool   `json:"authorized"`
	Connected  bool   `json:"connected"`
	Devices    int    `json:"devices"`
	Error      string `json:"error,omitempty"`
}

func (s *Server) eventStatus(w http.ResponseWriter, _ *http.Request) {
	s.hub.mu.Lock()
	status := EventStatus{Enabled: s.options.OpenEvents != nil, Devices: s.hub.devices, Error: s.hub.lastError}
	if s.hub.stream != nil {
		status.Connected = s.hub.stream.Connected()
	}
	s.hub.mu.Unlock()
	if status.Enabled {
		status.Authorized = s.service.OAuthReady()
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) oauthURL(w http.ResponseWriter, _ *http.Request) {
	address, err := s.service.OAuthURL()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": address})
}

func (s *Server) oauthComplete(w http.ResponseWriter, r *http.Request) {
	var input struct {
		URL string `json:"url"`
	}
	if !decode(w, r, &input) {
		return
	}
	if err := s.service.CompleteOAuth(input.URL); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// Start without waiting for the next retry.
	select {
	case s.authorized <- struct{}{}:
	default:
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authorized": true})
}

// events streams events as Server-Sent Events. Clients resume with Last-Event-ID or
// ?after=<seq>; without either they only get new events.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming is unsupported"))
		return
	}
	cursor := r.Header.Get("Last-Event-ID")
	if cursor == "" {
		cursor = r.URL.Query().Get("after")
	}
	after := ^uint64(0)
	if cursor != "" {
		parsed, err := strconv.ParseUint(cursor, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid sequence number"))
			return
		}
		after = parsed
	}
	backlog, subscriber, cancel := s.hub.subscribe(after)
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	send := func(event Event) {
		payload, err := json.Marshal(event)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "id: %d\ndata: %s\n\n", event.Seq, payload)
	}
	for _, event := range backlog {
		send(event)
	}
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case event, open := <-subscriber:
			if !open {
				return
			}
			send(event)
			flusher.Flush()
		}
	}
}
