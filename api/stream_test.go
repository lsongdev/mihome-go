package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/lsongdev/mihome-go/mihome"
)

type cameraService struct{ *fake }

func (cameraService) HomeList() ([]mihome.Home, error) { return []mihome.Home{{Name: "home"}}, nil }

func (cameraService) HomeDevices(mihome.Home) ([]mihome.DeviceInfo, error) {
	return []mihome.DeviceInfo{
		{Did: "cam", Name: "camera", SpecType: "urn:miot-spec-v2:device:camera:0000A01C:chuangmi-ip029a:2"},
		{Did: "fan", Name: "fan", SpecType: "urn:miot-spec-v2:device:fan:0000A005:dmaker-p45:1"},
	}, nil
}

type fakeCamera struct {
	err   error
	did   string
	audio bool
}

func (c *fakeCamera) Stream(_ context.Context, did string, audio bool, w io.Writer, ready func(string)) error {
	c.did, c.audio = did, audio
	if c.err != nil {
		return c.err
	}
	ready(`video/mp4; codecs="hvc1"`)
	_, err := w.Write([]byte("video"))
	return err
}

func TestStream(t *testing.T) {
	camera := &fakeCamera{}
	s := New(cameraService{&fake{}}, Options{Camera: camera})
	w := call(s, "GET", "/devices/cam/stream", "")
	if w.Code != http.StatusOK || w.Body.String() != "video" || !strings.HasPrefix(w.Header().Get("Content-Type"), "video/mp4") {
		t.Fatalf("%d %s %v", w.Code, w.Body, w.Header())
	}
	if camera.did != "cam" || !camera.audio {
		t.Errorf("camera = %+v", camera)
	}
	if w := call(s, "GET", "/devices/cam/stream?audio=0", ""); w.Code != http.StatusOK || camera.audio {
		t.Errorf("%d, audio = %v", w.Code, camera.audio)
	}
}

func TestStreamOnlyServesCameras(t *testing.T) {
	camera := &fakeCamera{}
	s := New(cameraService{&fake{}}, Options{Camera: camera})
	for _, path := range []string{"/devices/fan/stream", "/devices/nope/stream"} {
		if w := call(s, "GET", path, ""); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d %s", path, w.Code, w.Body)
		}
	}
	if camera.did != "" {
		t.Errorf("streamed %s", camera.did)
	}
	if w := call(New(cameraService{&fake{}}, Options{}), "GET", "/devices/cam/stream", ""); w.Code != http.StatusNotFound {
		t.Errorf("without a camera: %d", w.Code)
	}
}

func TestStreamFailure(t *testing.T) {
	s := New(cameraService{&fake{}}, Options{Camera: &fakeCamera{err: errors.New("no route")}})
	w := call(s, "GET", "/devices/cam/stream", "")
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "no route") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
