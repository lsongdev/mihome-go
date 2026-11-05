// Package api is the JSON API over mihome, with routes relative to wherever
// it is mounted (package web mounts it at /api/). Authentication is the
// mounting handler's job.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lsongdev/mihome-go/mihome"
	"golang.org/x/sync/singleflight"
)

// Service is the part of mihome.Client the API uses.
type Service interface {
	HomeList() ([]mihome.Home, error)
	HomeDevices(home mihome.Home) ([]mihome.DeviceInfo, error)
	FetchSpec(urn string) (*mihome.Spec, error)
	OAuthReady() bool
	OAuthURL() (string, error)
	CompleteOAuth(redirected string) error
	MiotGetProps(did string, iids []mihome.Iid) ([]any, error)
	MiotSetProp(did string, iid mihome.Iid, value any) (any, error)
	MiotAction(did string, iid []int, in []any) (mihome.H, error)
	ListMinaDevices(master int) ([]mihome.DeviceData, error)
	GetConversationRecords(did string, limit int) ([]mihome.ConversationRecord, error)
	TextToSpeech(deviceId, text string) (mihome.H, error)
	PlayerSetVolume(deviceId string, volume int) (mihome.H, error)
	PlayerGetStatus(deviceId string) (any, error)
	PlayerPlay(deviceId string) (mihome.H, error)
	PlayerPause(deviceId string) (mihome.H, error)
	PlayerStop(deviceId string) (mihome.H, error)
}

// Options configures a Server. Without a Cache every request goes upstream.
type Options struct {
	Cache Cache
	// DevicesTTL is how long homes, devices and speakers are kept.
	DevicesTTL time.Duration
	// SpecTTL is how long device specifications are kept.
	SpecTTL time.Duration
	// OpenEvents connects the realtime stream. Without it there are no events.
	OpenEvents func(context.Context, mihome.EventOptions) (EventStream, error)
	// EventsIncludeShared also follows the homes shared with the account.
	EventsIncludeShared bool
	// Camera streams video. Without it there is no video.
	Camera Camera
}

// Camera writes a camera's live video as fragmented MP4 until ctx ends or the
// stream fails. It calls ready once, with the content type, before any output.
type Camera interface {
	Stream(ctx context.Context, did string, audio bool, w io.Writer, ready func(contentType string)) error
}

type Server struct {
	service Service
	options Options
	flight  singleflight.Group
	mux     *http.ServeMux
	hub     *eventHub
	// authorized wakes RunEvents after a login.
	authorized chan struct{}
}

func New(service Service, options Options) *Server {
	s := &Server{service: service, options: options, mux: http.NewServeMux(), hub: newEventHub(), authorized: make(chan struct{}, 1)}
	s.mux.HandleFunc("GET /events", s.events)
	s.mux.HandleFunc("GET /events/status", s.eventStatus)
	s.mux.HandleFunc("GET /oauth", s.oauthURL)
	s.mux.HandleFunc("POST /oauth", s.oauthComplete)
	s.mux.HandleFunc("GET /homes", s.homes)
	s.mux.HandleFunc("GET /devices/{did}", s.device)
	s.mux.HandleFunc("GET /devices/{did}/spec", s.spec)
	s.mux.HandleFunc("GET /devices/{did}/stream", s.stream)
	s.mux.HandleFunc("GET /devices/{did}/props", s.getProps)
	s.mux.HandleFunc("PUT /devices/{did}/props", s.setProp)
	s.mux.HandleFunc("POST /devices/{did}/actions", s.action)
	s.mux.HandleFunc("GET /speakers", s.speakers)
	s.mux.HandleFunc("GET /speakers/{id}", s.speaker)
	s.mux.HandleFunc("GET /speakers/{id}/conversations", s.conversations)
	s.mux.HandleFunc("GET /speakers/{id}/player", s.playerStatus)
	s.mux.HandleFunc("POST /speakers/{id}/player", s.playerControl)
	s.mux.HandleFunc("PUT /speakers/{id}/volume", s.volume)
	s.mux.HandleFunc("POST /speakers/{id}/tts", s.tts)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// Device and Speaker are what the API exposes of a device. They are allowlists
// on purpose: the upstream records also hold each device's local control token,
// its MAC address and its network details.
type Device struct {
	Did    string `json:"did"`
	Name   string `json:"name"`
	Model  string `json:"model"`
	Online bool   `json:"online"`
	Spec   string `json:"spec"`
}

type Home struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Shared  bool     `json:"shared"`
	Devices []Device `json:"devices"`
}

type Speaker struct {
	ID       string `json:"id"`
	MiotDID  string `json:"miot_did"`
	Name     string `json:"name"`
	Alias    string `json:"alias"`
	Hardware string `json:"hardware"`
	Online   bool   `json:"online"`
}

func newDevice(d mihome.DeviceInfo) Device {
	return Device{Did: d.Did, Name: d.Name, Model: d.Model, Online: d.IsOnline, Spec: d.SpecType}
}

func newSpeaker(d mihome.DeviceData) Speaker {
	return Speaker{ID: d.DeviceID, MiotDID: d.MiotDID, Name: d.Name, Alias: d.Alias, Hardware: d.Hardware, Online: d.Presence == "online"}
}

func (s *Server) loadHomes() ([]Home, error) {
	homes, err := s.service.HomeList()
	if err != nil {
		return nil, err
	}
	out := make([]Home, 0, len(homes))
	for _, home := range homes {
		devices, err := s.service.HomeDevices(home)
		if err != nil {
			return nil, err
		}
		list := make([]Device, 0, len(devices))
		for _, d := range devices {
			list = append(list, newDevice(d))
		}
		out = append(out, Home{ID: home.ID.String(), Name: home.Name, Shared: home.Shared, Devices: list})
	}
	return out, nil
}

func (s *Server) allHomes(r *http.Request) ([]Home, error) {
	return cached(s, "homes", s.options.DevicesTTL, wantsRefresh(r), false, s.loadHomes)
}

func (s *Server) homes(w http.ResponseWriter, r *http.Request) {
	homes, err := s.allHomes(r)
	respond(w, homes, err)
}

type deviceInHome struct {
	Device Device `json:"device"`
	Home   string `json:"home"`
	Shared bool   `json:"shared"`
}

func (s *Server) findDevice(r *http.Request) (deviceInHome, error) {
	homes, err := s.allHomes(r)
	if err != nil {
		return deviceInHome{}, err
	}
	did := r.PathValue("did")
	for _, home := range homes {
		for _, d := range home.Devices {
			if d.Did == did {
				return deviceInHome{Device: d, Home: home.Name, Shared: home.Shared}, nil
			}
		}
	}
	return deviceInHome{}, mihome.ErrDeviceNotFound
}

func (s *Server) device(w http.ResponseWriter, r *http.Request) {
	found, err := s.findDevice(r)
	respond(w, found, err)
}

// stream answers a camera's live video until the client disconnects. Add
// ?audio=0 for a stream without audio, for browsers that can't play its codec.
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	found, err := s.findDevice(r)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	if s.options.Camera == nil || !strings.Contains(found.Device.Spec, ":device:camera:") {
		writeError(w, http.StatusNotFound, errors.New("这台设备没有视频"))
		return
	}
	started := false
	err = s.options.Camera.Stream(r.Context(), found.Device.Did, r.URL.Query().Get("audio") != "0", w, func(contentType string) {
		started = true
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	})
	switch {
	case err == nil || r.Context().Err() != nil:
	case started:
		log.Printf("camera %s: %v", found.Device.Did, err)
	default:
		writeUpstreamError(w, err)
	}
}

// specCachePrefix has a version, so that a change to mihome.Spec does not leave
// entries without the new fields in place for a whole SpecTTL.
const specCachePrefix = "spec2:"

// spec answers the device's specification, or null if it has none.
func (s *Server) spec(w http.ResponseWriter, r *http.Request) {
	found, err := s.findDevice(r)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	urn := found.Device.Spec
	if urn == "" {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	// A specification hardly ever changes, so an old one beats none at all.
	spec, err := cached(s, specCachePrefix+urn, s.options.SpecTTL, false, true, func() (*mihome.Spec, error) {
		return s.service.FetchSpec(urn)
	})
	respond(w, spec, err)
}

// getProps reads MIoT properties, given as ?iid=<siid>.<piid>.
func (s *Server) getProps(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()["iid"]
	if len(values) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("iid is required, as <siid>.<piid>"))
		return
	}
	iids := make([]mihome.Iid, 0, len(values))
	for _, value := range values {
		iid, err := parseIid(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		iids = append(iids, iid)
	}
	props, err := s.service.MiotGetProps(r.PathValue("did"), iids)
	respond(w, props, err)
}

func parseIid(value string) (mihome.Iid, error) {
	service, property, found := strings.Cut(value, ".")
	siid, err1 := strconv.Atoi(service)
	piid, err2 := strconv.Atoi(property)
	if !found || err1 != nil || err2 != nil {
		return mihome.Iid{}, fmt.Errorf("invalid iid %q, want <siid>.<piid>", value)
	}
	return mihome.Iid{Siid: siid, Piid: piid}, nil
}

func (s *Server) setProp(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Siid  int `json:"siid"`
		Piid  int `json:"piid"`
		Value any `json:"value"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.Value == nil {
		writeError(w, http.StatusBadRequest, errors.New("value is required"))
		return
	}
	result, err := s.service.MiotSetProp(r.PathValue("did"), mihome.Iid{Siid: input.Siid, Piid: input.Piid}, input.Value)
	if err == nil {
		err = deviceError(result)
	}
	respond(w, result, err)
}

func (s *Server) action(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Siid int   `json:"siid"`
		Aiid int   `json:"aiid"`
		In   []any `json:"in"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.In == nil {
		input.In = []any{}
	}
	result, err := s.service.MiotAction(r.PathValue("did"), []int{input.Siid, input.Aiid}, input.In)
	if err == nil {
		err = deviceError(result)
	}
	respond(w, result, err)
}

func (s *Server) allSpeakers(r *http.Request) ([]Speaker, error) {
	return cached(s, "speakers", s.options.DevicesTTL, wantsRefresh(r), false, func() ([]Speaker, error) {
		devices, err := s.service.ListMinaDevices(1)
		if err != nil {
			return nil, err
		}
		list := make([]Speaker, 0, len(devices))
		for _, d := range devices {
			list = append(list, newSpeaker(d))
		}
		return list, nil
	})
}

func (s *Server) speakers(w http.ResponseWriter, r *http.Request) {
	list, err := s.allSpeakers(r)
	respond(w, list, err)
}

func (s *Server) speaker(w http.ResponseWriter, r *http.Request) {
	list, err := s.allSpeakers(r)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	for _, speaker := range list {
		if speaker.ID == r.PathValue("id") {
			writeJSON(w, http.StatusOK, speaker)
			return
		}
	}
	writeUpstreamError(w, mihome.ErrDeviceNotFound)
}

func (s *Server) conversations(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 200 {
			writeError(w, http.StatusBadRequest, errors.New("limit must be between 1 and 200"))
			return
		}
		limit = parsed
	}
	records, err := s.service.GetConversationRecords(r.PathValue("id"), limit)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	if records == nil {
		records = []mihome.ConversationRecord{}
	}
	writeJSON(w, http.StatusOK, records)
}

func (s *Server) playerStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.service.PlayerGetStatus(r.PathValue("id"))
	respond(w, status, err)
}

func (s *Server) playerControl(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Action string `json:"action"`
	}
	if !decode(w, r, &input) {
		return
	}
	var control func(string) (mihome.H, error)
	switch input.Action {
	case "play":
		control = s.service.PlayerPlay
	case "pause":
		control = s.service.PlayerPause
	case "stop":
		control = s.service.PlayerStop
	default:
		writeError(w, http.StatusBadRequest, errors.New("action must be play, pause or stop"))
		return
	}
	result, err := control(r.PathValue("id"))
	respond(w, result, err)
}

func (s *Server) volume(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Volume *int `json:"volume"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.Volume == nil || *input.Volume < 0 || *input.Volume > 100 {
		writeError(w, http.StatusBadRequest, errors.New("volume must be between 0 and 100"))
		return
	}
	result, err := s.service.PlayerSetVolume(r.PathValue("id"), *input.Volume)
	respond(w, result, err)
}

func (s *Server) tts(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.Text) == "" {
		writeError(w, http.StatusBadRequest, errors.New("text is required"))
		return
	}
	result, err := s.service.TextToSpeech(r.PathValue("id"), input.Text)
	respond(w, result, err)
}

// deviceError finds a failure in a set_properties or action result. The request
// itself succeeds even when the device refuses: MIoT reports that per property,
// with a negative code. A code of 1 means the device accepted it for later.
func deviceError(result any) error {
	switch r := result.(type) {
	case []any:
		for _, item := range r {
			if err := deviceError(item); err != nil {
				return err
			}
		}
	case map[string]any:
		if code, ok := r["code"].(float64); ok && code < 0 {
			return fmt.Errorf("设备返回错误码 %.0f", code)
		}
	}
	return nil
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func respond(w http.ResponseWriter, value any, err error) {
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

// writeUpstreamError reports a failure of the Xiaomi cloud as 502, never 401:
// 401 belongs to the authentication of this API.
func writeUpstreamError(w http.ResponseWriter, err error) {
	var verification *mihome.VerificationError
	switch {
	case errors.As(err, &verification):
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]any{
			"code":    "verification_required",
			"message": "小米账号需要安全验证",
			"fields":  map[string]string{"url": verification.URL},
		}})
	case errors.Is(err, mihome.ErrDeviceNotFound):
		writeError(w, http.StatusNotFound, err)
	default:
		writeError(w, http.StatusBadGateway, err)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
