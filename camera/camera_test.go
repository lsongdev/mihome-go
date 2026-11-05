package camera

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// fakeProducer is a camera with audio only: the muxer needs real parameter sets
// for video, and audio is enough to test the sessions.
type fakeProducer struct {
	core.Connection
	end     chan error
	stopped chan struct{}
	once    sync.Once
}

func newFakeProducer() *fakeProducer {
	return &fakeProducer{
		Connection: core.Connection{Medias: []*core.Media{{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{{Name: core.CodecPCMA, ClockRate: 8000}},
		}, {
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs:    []*core.Codec{{Name: core.CodecPCMA, ClockRate: 8000}},
		}}},
		end:     make(chan error, 1),
		stopped: make(chan struct{}),
	}
}

func (p *fakeProducer) Start() error { return <-p.end }

func (p *fakeProducer) Stop() error {
	p.once.Do(func() {
		close(p.stopped)
		p.end <- errors.New("stopped")
	})
	return p.Connection.Stop()
}

func wait(t *testing.T, c <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

type viewer struct {
	cancel context.CancelFunc
	ready  chan struct{}
	types  chan string
	result chan error
}

func watch(m *Manager, did string) *viewer {
	ctx, cancel := context.WithCancel(context.Background())
	v := &viewer{cancel, make(chan struct{}), make(chan string, 1), make(chan error, 1)}
	go func() {
		v.result <- m.Stream(ctx, did, true, io.Discard, func(contentType string) {
			v.types <- contentType
			close(v.ready)
		})
	}()
	return v
}

func (v *viewer) finished(t *testing.T) error {
	t.Helper()
	select {
	case err := <-v.result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the viewer to finish")
		return nil
	}
}

func TestViewersShareAConnection(t *testing.T) {
	producer := newFakeProducer()
	dials := make(chan string, 4)
	m := New(nil)
	m.connect = func(did string) (core.Producer, error) {
		dials <- did
		return producer, nil
	}
	first, second := watch(m, "d1"), watch(m, "d1")
	wait(t, first.ready, "the first viewer")
	wait(t, second.ready, "the second viewer")
	if contentType := <-first.types; !strings.HasPrefix(contentType, "video/mp4") || !strings.Contains(contentType, "flac") {
		t.Errorf("content type = %q", contentType)
	}
	if len(dials) != 1 {
		t.Errorf("dialed %d times", len(dials))
	}

	first.cancel()
	first.finished(t)
	select {
	case <-producer.stopped:
		t.Fatal("the connection closed with a viewer left")
	case <-time.After(50 * time.Millisecond):
	}

	second.cancel()
	second.finished(t)
	wait(t, producer.stopped, "the connection to close")
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sessions) != 0 {
		t.Errorf("sessions = %v", m.sessions)
	}
}

func TestFailedDialIsRetried(t *testing.T) {
	failure := errors.New("no route")
	dials := 0
	m := New(nil)
	m.connect = func(string) (core.Producer, error) {
		dials++
		if dials == 1 {
			return nil, failure
		}
		return newFakeProducer(), nil
	}
	first := watch(m, "d1")
	if err := first.finished(t); !errors.Is(err, failure) {
		t.Fatalf("err = %v", err)
	}
	second := watch(m, "d1")
	wait(t, second.ready, "the second viewer")
	second.cancel()
	second.finished(t)
	if dials != 2 {
		t.Errorf("dialed %d times", dials)
	}
}

func TestStreamEndReachesViewers(t *testing.T) {
	producer := newFakeProducer()
	m := New(nil)
	m.connect = func(string) (core.Producer, error) { return producer, nil }
	v := watch(m, "d1")
	defer v.cancel()
	wait(t, v.ready, "the viewer")
	failure := errors.New("camera went away")
	producer.end <- failure
	if err := v.finished(t); !errors.Is(err, failure) {
		t.Fatalf("err = %v", err)
	}
}

func TestViewerCanLeaveDuringDial(t *testing.T) {
	producer := newFakeProducer()
	dialing, proceed := make(chan struct{}), make(chan struct{})
	m := New(nil)
	m.connect = func(string) (core.Producer, error) {
		close(dialing)
		<-proceed
		return producer, nil
	}
	v := watch(m, "d1")
	wait(t, dialing, "the dial")
	v.cancel()
	if err := v.finished(t); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	close(proceed)
	wait(t, producer.stopped, "the abandoned connection to close")
}
