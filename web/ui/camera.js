// Live camera video: the API sends fragmented MP4, played through Media Source
// Extensions, because a plain <video src> can't follow a stream that never ends.
import { html, useEffect, useRef, useState } from './lib/html.js';
import { Alert } from './lib/ui.js';
import { api } from './mi.js';

const Source = window.ManagedMediaSource ?? window.MediaSource;
// Seconds of video to keep behind the playhead, and how far it may fall behind.
const KEEP_BEHIND = 10;
const MAX_LAG = 3;

// open returns the response of the first variant the browser can play: audio is
// dropped if that is what it can't decode.
async function open(api, path, signal) {
  for (const query of [{}, { audio: 0 }]) {
    const response = await fetch(api.url(`${path}/stream`, query), { signal });
    if (!response.ok) {
      const payload = await response.json().catch(() => null);
      throw new Error(payload?.error?.message ?? payload?.error ?? `请求失败：HTTP ${response.status}`);
    }
    if (Source.isTypeSupported(response.headers.get('Content-Type'))) return response;
    await response.body.cancel();
  }
  throw new Error('这个浏览器无法解码摄像机的视频（通常是 H.265）');
}

function appendTo(buffer, chunk, signal) {
  return new Promise((resolve, reject) => {
    const finish = () => {
      buffer.removeEventListener('updateend', done);
      buffer.removeEventListener('error', failed);
      signal.removeEventListener('abort', done);
    };
    const done = () => { finish(); resolve(); };
    const failed = () => { finish(); reject(new Error('浏览器无法解码这段视频')); };
    buffer.addEventListener('updateend', done);
    buffer.addEventListener('error', failed);
    signal.addEventListener('abort', done);
    buffer.appendBuffer(chunk);
  });
}

// trim stays near the live edge and frees what has already played.
function trim(video, buffer) {
  if (!buffer.buffered.length || buffer.updating) return;
  const end = buffer.buffered.end(buffer.buffered.length - 1);
  if (end - video.currentTime > MAX_LAG) video.currentTime = end - 0.5;
  const before = video.currentTime - KEEP_BEHIND;
  if (before > buffer.buffered.start(0)) buffer.remove(buffer.buffered.start(0), before);
}

async function play(video, response, signal, onPlaying) {
  const media = new Source();
  const source = URL.createObjectURL(media);
  video.disableRemotePlayback = true;
  video.addEventListener('playing', onPlaying, { once: true });
  video.src = source;
  try {
    await new Promise(resolve => media.addEventListener('sourceopen', resolve, { once: true }));
    const buffer = media.addSourceBuffer(response.headers.get('Content-Type'));
    const reader = response.body.getReader();
    for (;;) {
      const { value, done } = await reader.read();
      if (done) throw new Error('摄像机的视频流已断开');
      if (signal.aborted) return;
      if (buffer.updating) await new Promise(resolve => buffer.addEventListener('updateend', resolve, { once: true }));
      await appendTo(buffer, value, signal);
      if (signal.aborted) return;
      // Not awaited: play() only resolves once more video has been appended.
      if (video.paused && buffer.buffered.length) video.play().catch(() => {});
      trim(video, buffer);
    }
  } finally {
    video.removeEventListener('playing', onPlaying);
    video.removeAttribute('src');
    video.load();
    URL.revokeObjectURL(source);
  }
}

// CameraView only connects on request: a connection can show on the camera, and
// each one takes resources from it.
export function CameraView({ path, online }) {
  const video = useRef(null);
  const [state, setState] = useState('stopped');
  const [error, setError] = useState('');
  const wanted = state !== 'stopped';

  useEffect(() => {
    if (!wanted) return undefined;
    const controller = new AbortController();
    const { signal } = controller;
    (async () => {
      if (!Source) throw new Error('这个浏览器不支持 Media Source Extensions，无法播放视频');
      const response = await open(api, path, signal);
      await play(video.current, response, signal, () => setState('playing'));
    })().catch(err => {
      if (signal.aborted) return;
      setError(err.message);
      setState('stopped');
    });
    return () => controller.abort();
  }, [wanted, path]);

  const toggle = () => {
    setError('');
    setState(wanted ? 'stopped' : 'connecting');
  };
  return html`<section class="section">
    <h2>实时画面</h2>
    <div class="card camera">
      ${error ? html`<${Alert} variant="danger">${error}<//>` : null}
      <video ref=${video} class="camera-video" hidden=${!wanted} muted autoplay playsinline controls></video>
      <div class="camera-actions">
        ${state === 'connecting' ? html`<span class="muted">正在连接摄像机…</span>` : null}
        <button class="button" disabled=${!online && !wanted} onClick=${toggle}>${wanted ? '停止' : '开始播放'}</button>
      </div>
    </div>
  </section>`;
}
