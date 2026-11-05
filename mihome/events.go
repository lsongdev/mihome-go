package mihome

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lsongdev/mqtt-go/mqtt"
	"github.com/lsongdev/mqtt-go/proto"
)

const (
	eventBroker    = "cn-ha.mqtt.io.mi.com:8883"
	eventKeepAlive = 60
	// The broker has no wildcard for all devices: each one needs its own topics.
	eventSubscribeBatch = 60
)

// EventType says what an Event reports.
type EventType string

const (
	// EventProperty is a changed property: Siid, Piid and Value.
	EventProperty EventType = "property"
	// EventOccurred is a device event, such as a button press: Siid, Eiid and
	// Arguments.
	EventOccurred EventType = "event"
	// EventAvailability is a device going online or offline: Online.
	EventAvailability EventType = "availability"
)

// Event is one realtime message from a device.
type Event struct {
	Type      EventType
	DeviceID  string
	Siid      int
	Piid      int
	Eiid      int
	Value     any
	Arguments any
	Online    bool
	Time      time.Time
}

// EventOptions are the hooks of an EventStream. They run on its connection
// goroutine and must not block for long.
type EventOptions struct {
	// OnConnect runs after a reconnect. Changes made during the outage were not
	// delivered, so this is the time to read current state again.
	OnConnect func()
	// OnDisconnect runs when the connection is lost.
	OnDisconnect func()
	// OnError reports failed reconnection attempts, and devices refused on
	// reconnect.
	OnError func(error)

	broker string
	tls    *tls.Config
}

// EventStream delivers the realtime events of a set of devices, through the cloud
// MQTT broker. It reconnects on its own, with a fresh token each time.
type EventStream struct {
	client *mqtt.ReconnectingClient
	events chan Event
	cancel context.CancelFunc
	done   chan struct{}

	mu      sync.Mutex
	devices map[string]bool
}

// OpenEvents connects to the broker. It returns ErrOAuthRequired until the login of
// OAuthURL is done. Call SetDevices to choose the devices.
func (ma *Client) OpenEvents(ctx context.Context, options EventOptions) (*EventStream, error) {
	_, token, err := ma.oauthAccessToken()
	if err != nil {
		return nil, err
	}
	broker, tlsConfig := options.broker, options.tls
	if broker == "" {
		broker, tlsConfig = eventBroker, &tls.Config{MinVersion: tls.VersionTLS12}
	}
	client, err := mqtt.DialWithReconnect(ctx, broker, mqtt.ClientOptions{
		ProtocolVersion: proto.Version5,
		ClientID:        token.deviceID(),
		CleanStart:      true,
		KeepAlive:       eventKeepAlive,
		EnableQoS2:      true,
		TLSConfig:       tlsConfig,
		Credentials: func(context.Context) (string, string, error) {
			access, _, err := ma.oauthAccessToken()
			return ma.OAuthClientID, access, err
		},
	}, mqtt.ReconnectOptions{
		MinDelay:     time.Second,
		MaxDelay:     5 * time.Minute,
		OnConnect:    options.OnConnect,
		OnDisconnect: options.OnDisconnect,
		OnError:      options.OnError,
	})
	if err != nil {
		return nil, err
	}
	run, cancel := context.WithCancel(context.Background())
	stream := &EventStream{
		client:  client,
		events:  make(chan Event, 256),
		cancel:  cancel,
		done:    make(chan struct{}),
		devices: make(map[string]bool),
	}
	go stream.run(run)
	return stream, nil
}

func (s *EventStream) run(ctx context.Context) {
	defer close(s.done)
	defer close(s.events)
	for message := range s.client.Incoming {
		event, err := parseEvent(message.TopicName, message.Bytes())
		if err != nil {
			continue
		}
		event.Time = time.Now()
		select {
		case s.events <- event:
		case <-ctx.Done():
			return
		}
	}
}

// Events returns the channel of events. It is closed by Close.
func (s *EventStream) Events() <-chan Event { return s.events }

// Connected reports whether the connection is up right now.
func (s *EventStream) Connected() bool { return s.client.Connected() }

// Close stops the stream.
func (s *EventStream) Close() error {
	s.cancel()
	err := s.client.Close()
	<-s.done
	return err
}

func eventTopics(did string) []proto.TopicQos {
	topics := []proto.TopicQos{
		{Topic: "device/" + did + "/up/properties_changed/#", Qos: proto.QosExactlyOnce},
		{Topic: "device/" + did + "/up/event_occured/#", Qos: proto.QosExactlyOnce},
	}
	// The cloud may not report availability for these kinds of device.
	if !strings.HasPrefix(did, "blt.") && !strings.HasPrefix(did, "proxy.") {
		topics = append(topics, proto.TopicQos{Topic: "device/" + did + "/state/#", Qos: proto.QosExactlyOnce})
	}
	return topics
}

// SetDevices makes the stream follow exactly these devices. Devices the broker
// refuses are reported with an error naming them; the others are still followed.
func (s *EventStream) SetDevices(ctx context.Context, dids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := make(map[string]bool, len(dids))
	var add []proto.TopicQos
	for _, did := range dids {
		want[did] = true
		if !s.devices[did] {
			add = append(add, eventTopics(did)...)
		}
	}
	var remove []string
	for did := range s.devices {
		if !want[did] {
			for _, topic := range eventTopics(did) {
				remove = append(remove, topic.Topic)
			}
		}
	}
	if len(remove) > 0 {
		if _, err := s.client.UnsubscribeContext(ctx, remove); err != nil {
			return err
		}
	}
	refused := map[string]bool{}
	for len(add) > 0 {
		n := min(len(add), eventSubscribeBatch)
		_, err := s.client.SubscribeContext(ctx, add[:n])
		var denied *mqtt.SubscribeError
		switch {
		case errors.As(err, &denied):
			for _, topic := range denied.Denied {
				refused[strings.Split(topic.Topic, "/")[1]] = true
			}
		case err != nil:
			return err
		}
		add = add[n:]
	}
	s.devices = make(map[string]bool, len(want))
	for did := range want {
		if !refused[did] {
			s.devices[did] = true
		}
	}
	if len(refused) > 0 {
		names := make([]string, 0, len(refused))
		for did := range refused {
			names = append(names, did)
		}
		return fmt.Errorf("broker 拒绝订阅 %d 台设备: %s", len(names), strings.Join(names, ", "))
	}
	return nil
}

// parseEvent reads one broker message. The topics are:
//
//	device/{did}/up/properties_changed/{siid}/{piid}
//	device/{did}/up/event_occured/{siid}/{eiid} (the broker's own spelling)
//	device/{did}/state/...
func parseEvent(topic string, payload []byte) (Event, error) {
	parts := strings.Split(topic, "/")
	if len(parts) < 3 || parts[0] != "device" {
		return Event{}, fmt.Errorf("unknown topic %q", topic)
	}
	event := Event{DeviceID: parts[1]}
	if parts[2] == "state" {
		var state struct {
			Event string `json:"event"`
		}
		if err := json.Unmarshal(payload, &state); err != nil {
			return Event{}, err
		}
		if state.Event == "" {
			return Event{}, fmt.Errorf("no state in %q", topic)
		}
		event.Type, event.Online = EventAvailability, state.Event == "online"
		return event, nil
	}
	if len(parts) < 4 || parts[2] != "up" {
		return Event{}, fmt.Errorf("unknown topic %q", topic)
	}
	var message struct {
		Params struct {
			Siid      *int `json:"siid"`
			Piid      *int `json:"piid"`
			Eiid      *int `json:"eiid"`
			Value     any  `json:"value"`
			Arguments any  `json:"arguments"`
		} `json:"params"`
	}
	if err := json.Unmarshal(payload, &message); err != nil {
		return Event{}, err
	}
	params := message.Params
	switch {
	case parts[3] == "properties_changed" && params.Siid != nil && params.Piid != nil:
		event.Type, event.Siid, event.Piid, event.Value = EventProperty, *params.Siid, *params.Piid, params.Value
	case parts[3] == "event_occured" && params.Siid != nil && params.Eiid != nil:
		event.Type, event.Siid, event.Eiid, event.Arguments = EventOccurred, *params.Siid, *params.Eiid, params.Arguments
	default:
		return Event{}, fmt.Errorf("unknown message on %q: %s", topic, strconv.Itoa(len(payload)))
	}
	return event, nil
}
