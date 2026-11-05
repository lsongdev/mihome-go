package mihome

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lsongdev/mqtt-go/mqtt"
	"github.com/lsongdev/mqtt-go/proto"
)

func TestParseEvent(t *testing.T) {
	event, err := parseEvent("device/48260584/up/properties_changed/2/1",
		[]byte(`{"method":"properties_changed","params":{"value":true,"did":"48260584","siid":2,"piid":1},"last_timestamp":1,"last_value":false}`))
	if err != nil || event.Type != EventProperty || event.DeviceID != "48260584" || event.Siid != 2 || event.Piid != 1 || event.Value != true {
		t.Fatalf("%+v %v", event, err)
	}
	event, err = parseEvent("device/lumi.1/up/event_occured/3/1", []byte(`{"params":{"siid":3,"eiid":1,"arguments":[{"piid":1,"value":2}]}}`))
	if err != nil || event.Type != EventOccurred || event.Eiid != 1 || event.Arguments == nil {
		t.Fatalf("%+v %v", event, err)
	}
	event, err = parseEvent("device/9/state/x", []byte(`{"device_id":"9","event":"offline"}`))
	if err != nil || event.Type != EventAvailability || event.Online {
		t.Fatalf("%+v %v", event, err)
	}
	// A false value is a value: only a missing identifier makes a message invalid.
	event, err = parseEvent("device/9/up/properties_changed/2/1", []byte(`{"params":{"siid":2,"piid":1,"value":false}}`))
	if err != nil || event.Value != false {
		t.Fatalf("%+v %v", event, err)
	}
	for topic, payload := range map[string]string{
		"other/9/x":                          `{}`,
		"device/9":                           `{}`,
		"device/9/up/properties_changed/2/1": `{"params":{"value":1}}`,
		"device/9/up/event_occured/2/1":      `not json`,
		"device/9/state/x":                   `{}`,
		"device/9/up/unknown/1/1":            `{"params":{"siid":1,"piid":1}}`,
	} {
		if _, err := parseEvent(topic, []byte(payload)); err == nil {
			t.Errorf("%s: expected an error", topic)
		}
	}
}

func TestEventTopicsSkipAvailabilityOfProxiedDevices(t *testing.T) {
	if len(eventTopics("123")) != 3 || len(eventTopics("blt.3.x")) != 2 || len(eventTopics("proxy.1.2")) != 2 {
		t.Fatal("wrong topics")
	}
}

func oauthServer(t *testing.T, requests *[]map[string]string) *Client {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	client := newTestClient(t, path, "user", "password", func(req *http.Request) *http.Response {
		if req.URL.Host != "ha.api.io.mi.com" {
			t.Errorf("unexpected host %s", req.URL.Host)
		}
		*requests = append(*requests, map[string]string{"data": req.URL.Query().Get("data"), "ua": req.Header.Get("User-Agent")})
		return response(200, `{"code":0,"result":{"access_token":"access-`+string(rune('0'+len(*requests)))+`","refresh_token":"refresh","expires_in":1000}}`, "")
	})
	client.OAuthClientID = "12345"
	return client
}

func TestOAuthFlow(t *testing.T) {
	var requests []map[string]string
	client := oauthServer(t, &requests)
	if client.OAuthReady() {
		t.Fatal("ready before login")
	}
	if _, _, err := client.oauthAccessToken(); !errors.Is(err, ErrOAuthRequired) {
		t.Fatalf("err = %v", err)
	}
	address, err := client.OAuthURL()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.Host != "account.xiaomi.com" || query.Get("client_id") != "12345" || !strings.HasPrefix(query.Get("device_id"), "ha.") {
		t.Fatalf("url = %s", address)
	}
	// The same login stays valid across calls.
	again, _ := client.OAuthURL()
	if again != address {
		t.Fatal("login changed")
	}
	for name, bad := range map[string]string{
		"no code":     query.Get("redirect_uri") + "?state=" + query.Get("state"),
		"wrong state": query.Get("redirect_uri") + "?code=c&state=other",
		"not a url":   "::",
	} {
		if err := client.CompleteOAuth(bad); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if len(requests) != 0 {
		t.Fatal("a bad address reached the server")
	}
	if err := client.CompleteOAuth(" " + query.Get("redirect_uri") + "?code=the-code&state=" + query.Get("state") + "\n"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(requests[0]["data"], `"client_id":12345`) || !strings.Contains(requests[0]["data"], `"code":"the-code"`) ||
		!strings.Contains(requests[0]["ua"], query.Get("device_id")) {
		t.Fatalf("request = %v", requests[0])
	}
	if !client.OAuthReady() {
		t.Fatal("not ready after login")
	}
	access, _, err := client.oauthAccessToken()
	if err != nil || access != "access-1" || len(requests) != 1 {
		t.Fatalf("%q %v %d", access, err, len(requests))
	}
	// Renewal uses the refresh token, and the login survives a restart.
	client.Token.OAuth.RenewAt = time.Now().Add(-time.Second)
	access, _, err = client.oauthAccessToken()
	if err != nil || access != "access-2" || !strings.Contains(requests[1]["data"], `"refresh_token":"refresh"`) {
		t.Fatalf("%q %v %v", access, err, requests)
	}
	if client.Token.OAuth.RenewAt.Before(time.Now().Add(600 * time.Second)) {
		t.Fatalf("renew at %v", client.Token.OAuth.RenewAt)
	}
	reloaded := newTestClient(t, client.TokenPath, "user", "password", nil)
	if !reloaded.OAuthReady() {
		t.Fatal("login was not saved")
	}
	info, err := os.Stat(client.TokenPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("%v %v", info, err)
	}
}

func TestOAuthNeedsAClientID(t *testing.T) {
	client := newTestClient(t, "", "user", "password", nil)
	if _, err := client.OAuthURL(); err == nil {
		t.Fatal("expected an error")
	}
}

func TestOAuthErrorsDoNotExposeTokens(t *testing.T) {
	client := newTestClient(t, "", "user", "password", func(req *http.Request) *http.Response {
		return response(200, `{"code":-1,"message":"expired"}`, "")
	})
	client.OAuthClientID = "1"
	client.Token = NewTokens()
	client.Token.OAuth = &OAuthToken{UUID: "u", VirtualDid: "1", RefreshToken: "secret-refresh", RenewAt: time.Now().Add(-time.Hour)}
	_, _, err := client.oauthAccessToken()
	if err == nil || strings.Contains(err.Error(), "secret-refresh") || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("err = %v", err)
	}
}

func testBroker(t *testing.T) (*mqtt.Server, string, *[]string) {
	t.Helper()
	passwords := &[]string{}
	server, err := mqtt.NewServerWithOptions(mqtt.ServerOptions{
		EnableQoS2: true,
		Authenticator: mqtt.AuthenticateFunc(func(_ context.Context, req mqtt.AuthRequest) error {
			*passwords = append(*passwords, req.Username+":"+string(req.Password))
			return nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close(); server.Close() })
	go func() { _ = server.Serve(listener) }()
	return server, listener.Addr().String(), passwords
}

func nextEvent(t *testing.T, stream *EventStream) Event {
	t.Helper()
	select {
	case event := <-stream.Events():
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("no event")
		return Event{}
	}
}

func TestEventStream(t *testing.T) {
	_, address, passwords := testBroker(t)
	client := newTestClient(t, "", "user", "password", nil)
	client.OAuthClientID = "12345"
	client.Token = NewTokens()
	client.Token.OAuth = &OAuthToken{UUID: "u", VirtualDid: "1", AccessToken: "access", RefreshToken: "r", RenewAt: time.Now().Add(time.Hour)}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := client.OpenEvents(ctx, EventOptions{broker: address})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if (*passwords)[0] != "12345:access" || !stream.Connected() {
		t.Fatalf("credentials = %v", *passwords)
	}
	if err := stream.SetDevices(ctx, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	publisher, err := mqtt.Dial(ctx, address, mqtt.ClientOptions{ClientID: "publisher", CleanStart: true})
	if err != nil {
		t.Fatal(err)
	}
	defer publisher.Disconnect()
	publish := func(topic, body string) {
		t.Helper()
		if err := publisher.Publish(&proto.Publish{TopicName: topic, Payload: proto.BytesPayload(body)}); err != nil {
			t.Fatal(err)
		}
	}
	publish("device/other/up/properties_changed/2/1", `{"params":{"siid":2,"piid":1,"value":0}}`)
	publish("device/a/up/properties_changed/2/1", `{"params":{"siid":2,"piid":1,"value":7}}`)
	if event := nextEvent(t, stream); event.DeviceID != "a" || event.Value != float64(7) || event.Time.IsZero() {
		t.Fatalf("%+v", event)
	}
	// Dropping a device stops its events and keeps the others.
	if err := stream.SetDevices(ctx, []string{"b"}); err != nil {
		t.Fatal(err)
	}
	publish("device/a/up/properties_changed/2/1", `{"params":{"siid":2,"piid":1,"value":8}}`)
	publish("device/b/state/x", `{"event":"online"}`)
	if event := nextEvent(t, stream); event.DeviceID != "b" || !event.Online {
		t.Fatalf("%+v", event)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if _, open := <-stream.Events(); open {
		t.Fatal("events are still open")
	}
}

func TestOpenEventsBeforeLogin(t *testing.T) {
	client := newTestClient(t, "", "user", "password", nil)
	if _, err := client.OpenEvents(context.Background(), EventOptions{}); !errors.Is(err, ErrOAuthRequired) {
		t.Fatalf("err = %v", err)
	}
}
