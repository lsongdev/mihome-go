// Package camera streams Mi Home cameras as fragmented MP4, with the packages of
// go2rtc. Xiaomi's cloud gives out the keys for each connection; the video comes
// directly from the camera over the LAN.
package camera

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/crypto"
)

const (
	apiBase = "https://api.io.mi.com/app"
	apiSid  = "xiaomiio"
)

// Manager keeps one connection per camera, shared by the viewers of that camera.
type Manager struct {
	credentials func() (userID, passToken string, err error)
	// connect is replaced in tests.
	connect func(did string) (core.Producer, error)

	mu       sync.Mutex
	sessions map[string]*session

	cloudMu sync.Mutex
	cloud   *xiaomi.Cloud
}

// New returns a Manager that logs in to Xiaomi's cloud with credentials.
func New(credentials func() (userID, passToken string, err error)) *Manager {
	m := &Manager{credentials: credentials, sessions: map[string]*session{}}
	m.connect = m.dial
	return m
}

// track is an output of a camera.
type track struct {
	media    *core.Media
	receiver *core.Receiver
}

// session is one connection to a camera. mu guards refs.
type session struct {
	refs     int
	producer core.Producer
	tracks   []track
	// ready closes when the connection is up or has failed with err.
	ready chan struct{}
	err   error
	// done closes when the stream has ended, with endErr set.
	done   chan struct{}
	endErr error
}

// Stream writes a camera's live video to w until ctx ends or the stream fails. It
// calls ready once, with the content type, before any output.
func (m *Manager) Stream(ctx context.Context, did string, audio bool, w io.Writer, ready func(contentType string)) error {
	s := m.acquire(did)
	defer m.release(did, s)
	select {
	case <-s.ready:
	case <-ctx.Done():
		return ctx.Err()
	}
	if s.err != nil {
		return s.err
	}

	consumer := mp4.NewConsumer(consumerMedias(audio))
	defer consumer.Stop()
	if err := addTracks(consumer, s.tracks); err != nil {
		return err
	}
	ready(mp4.ContentType(consumer.Codecs()))

	output := &stoppableWriter{Writer: w}
	finished := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
		case <-s.done:
		case <-finished:
			return
		}
		// WriteTo only returns when its buffer closes or a write fails. Closing it
		// from here would race with WriteTo, so make it fail a write: an empty one
		// through the buffer, as there may be no more video. That does nothing
		// until WriteTo has started, so keep trying.
		output.stopped.Store(true)
		buffer := consumer.Transport.(io.Writer)
		for {
			_, _ = buffer.Write(nil)
			select {
			case <-finished:
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}()
	_, err := consumer.WriteTo(output)
	close(finished)
	select {
	case <-s.done:
		return s.endErr
	default:
		return err
	}
}

// stoppableWriter fails every write after it is stopped.
type stoppableWriter struct {
	io.Writer
	stopped atomic.Bool
}

func (w *stoppableWriter) Write(p []byte) (int, error) {
	if w.stopped.Load() {
		return 0, errors.New("stopped")
	}
	return w.Writer.Write(p)
}

// Flush keeps the video moving: go2rtc flushes writers that can be.
func (w *stoppableWriter) Flush() {
	if flusher, ok := w.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

// setup serializes addTracks: go2rtc's FLAC encoder initializes global tables
// without a lock the first time it is used.
var setup sync.Mutex

func addTracks(consumer *mp4.Consumer, tracks []track) error {
	setup.Lock()
	defer setup.Unlock()
	for _, media := range consumer.GetMedias() {
		for _, t := range tracks {
			if codec, wanted := t.media.MatchMedia(media); codec != nil {
				if err := consumer.AddTrack(media, wanted, t.receiver); err != nil {
					return err
				}
				break
			}
		}
	}
	if len(consumer.Senders) == 0 {
		return errors.New("摄像机的视频格式不受支持")
	}
	return nil
}

// consumerMedias lists the codecs that a browser can get. The muxer converts
// G.711 audio to FLAC.
func consumerMedias(audio bool) []*core.Media {
	medias := []*core.Media{{
		Kind:      core.KindVideo,
		Direction: core.DirectionSendonly,
		Codecs:    []*core.Codec{{Name: core.CodecH264}, {Name: core.CodecH265}},
	}}
	if audio {
		medias = append(medias, &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecAAC}, {Name: core.CodecOpus}, {Name: core.CodecPCMA}, {Name: core.CodecPCMU},
			},
		})
	}
	return medias
}

// acquire joins the camera's session, starting one if there is none.
func (m *Manager) acquire(did string) *session {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.sessions[did]; s != nil {
		s.refs++
		return s
	}
	s := &session{refs: 1, ready: make(chan struct{}), done: make(chan struct{})}
	m.sessions[did] = s
	go m.run(did, s)
	return s
}

// release leaves a session. The last viewer closes the connection.
func (m *Manager) release(did string, s *session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.refs--
	if s.refs > 0 {
		return
	}
	m.forget(did, s)
	if s.producer != nil {
		_ = s.producer.Stop()
	}
}

// forget must be called with mu held. A failed session may have been replaced.
func (m *Manager) forget(did string, s *session) {
	if m.sessions[did] == s {
		delete(m.sessions, did)
	}
}

func (m *Manager) run(did string, s *session) {
	producer, err := m.connect(did)
	var tracks []track
	if err == nil {
		tracks, err = recvTracks(producer)
		if err != nil {
			_ = producer.Stop()
		}
	}
	m.mu.Lock()
	s.err, s.tracks = err, tracks
	if err != nil {
		m.forget(did, s)
	} else if s.refs > 0 {
		s.producer = producer
	}
	abandoned := err == nil && s.refs == 0
	m.mu.Unlock()
	close(s.ready)
	if err != nil {
		return
	}
	if abandoned {
		_ = producer.Stop()
		return
	}
	s.endErr = producer.Start()
	if s.endErr == nil {
		s.endErr = errors.New("摄像机的视频流已结束")
	}
	m.mu.Lock()
	m.forget(did, s)
	m.mu.Unlock()
	close(s.done)
}

func recvTracks(producer core.Producer) ([]track, error) {
	var tracks []track
	for _, media := range producer.GetMedias() {
		if media.Direction != core.DirectionRecvonly {
			continue
		}
		receiver, err := producer.GetTrack(media, media.Codecs[0])
		if err != nil {
			return nil, err
		}
		tracks = append(tracks, track{media, receiver})
	}
	return tracks, nil
}

// dial connects to a camera. The URL holds the connection's keys: don't log it.
func (m *Manager) dial(did string) (core.Producer, error) {
	res, err := m.request("/v2/home/device_list_page", "{}")
	if err != nil {
		return nil, fmt.Errorf("获取设备列表失败: %w", err)
	}
	var list struct {
		List []struct {
			Did   string `json:"did"`
			Model string `json:"model"`
			IP    string `json:"localip"`
		} `json:"list"`
	}
	if err := json.Unmarshal(res, &list); err != nil {
		return nil, fmt.Errorf("解析设备列表失败: %w", err)
	}
	var ip, model string
	for _, device := range list.List {
		if device.Did == did {
			ip, model = device.IP, device.Model
		}
	}
	if ip == "" {
		return nil, errors.New("小米云没有返回摄像机的局域网地址")
	}

	public, private, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}
	params, err := json.Marshal(map[string]string{
		"app_pubkey":      hex.EncodeToString(public),
		"did":             did,
		"support_vendors": "TUTK_CS2_MTP",
	})
	if err != nil {
		return nil, err
	}
	res, err = m.request("/v2/device/miss_get_vendor", string(params))
	if err != nil {
		return nil, fmt.Errorf("获取摄像机连接密钥失败: %w", err)
	}
	var vendor struct {
		Vendor struct {
			ID     byte `json:"vendor"`
			Params struct {
				UID string `json:"p2p_id"`
			} `json:"vendor_params"`
		} `json:"vendor"`
		PublicKey string `json:"public_key"`
		Sign      string `json:"sign"`
	}
	if err := json.Unmarshal(res, &vendor); err != nil {
		return nil, fmt.Errorf("解析摄像机连接密钥失败: %w", err)
	}
	query := url.Values{
		"did":            {did},
		"model":          {model},
		"client_public":  {hex.EncodeToString(public)},
		"client_private": {hex.EncodeToString(private)},
		"device_public":  {vendor.PublicKey},
		"sign":           {vendor.Sign},
	}
	switch vendor.Vendor.ID {
	case 1:
		query.Set("vendor", "tutk")
		query.Set("uid", vendor.Vendor.Params.UID)
	case 4:
		query.Set("vendor", "cs2")
	default:
		return nil, fmt.Errorf("不支持这台摄像机的连接方式 %d", vendor.Vendor.ID)
	}
	producer, err := xiaomi.Dial((&url.URL{Scheme: "xiaomi", Host: ip, RawQuery: query.Encode()}).String())
	if err != nil {
		return nil, fmt.Errorf("连接摄像机 %s 失败: %w", ip, err)
	}
	return producer, nil
}

// request calls Xiaomi's cloud, logging in again if an existing login fails.
func (m *Manager) request(path, params string) ([]byte, error) {
	m.cloudMu.Lock()
	defer m.cloudMu.Unlock()
	if m.cloud != nil {
		if res, err := m.cloud.Request(apiBase, path, params, nil); err == nil {
			return res, nil
		}
		m.cloud = nil
	}
	userID, passToken, err := m.credentials()
	if err != nil {
		return nil, err
	}
	cloud := xiaomi.NewCloud(apiSid)
	if err := cloud.LoginWithToken(userID, passToken); err != nil {
		return nil, err
	}
	res, err := cloud.Request(apiBase, path, params, nil)
	if err != nil {
		return nil, err
	}
	m.cloud = cloud
	return res, nil
}
