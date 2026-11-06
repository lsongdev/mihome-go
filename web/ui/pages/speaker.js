import { html, useEffect, useState } from '../lib/html.js';
import { useAction, useResource } from '../lib/hooks.js';
import { PageHeader, Loading, Empty, KeyValue, toast } from '../lib/ui.js';
import { href } from '../lib/router.js';
import { api, useApi, useCached, useAvailability, ApiNotice, OnlineStatus, dateTime, playerInfo } from '../mi.js';
import { displaySpec } from '../lib/spec.js';
import { speakerControls } from '../lib/speaker.js';
import { DeviceControls } from '../controls.js';

export default function Speaker({ params }) {
  const path = `/speakers/${encodeURIComponent(params.id)}`;
  const speaker = useCached(path);
  const online = useAvailability(speaker.refresh);
  const back = { href: href('/speakers'), label: '音箱' };
  if (speaker.data === undefined) {
    return html`<${PageHeader} title="音箱" back=${back} />
      ${speaker.error ? html`<${ApiNotice} error=${speaker.error} />` : html`<${Loading} />`}`;
  }
  const s = { ...speaker.data, online: online(speaker.data.miot_did, speaker.data.online) };
  return html`<${PageHeader} title=${s.name} back=${back} subtitle=${html`<${OnlineStatus} online=${s.online} />`} />
    <section class="section">
      <h2>设备信息</h2>
      <div class="card"><${KeyValue} items=${[['型号', s.hardware], ['别名', s.alias], ['DID', html`<code>${s.miot_did}</code>`]]} /></div>
    </section>
    <${SpeakerActions} key=${s.id} path=${path} speaker=${s} />
    <section class="section"><h2>对话记录</h2><${Conversations} path=${path} /></section>`;
}

function SpeakerActions({ path, speaker }) {
  const devicePath = `/devices/${encodeURIComponent(speaker.miot_did)}`;
  const resource = useResource(signal => speaker.miot_did
    ? api.get(`${devicePath}/spec`, { signal }) : Promise.resolve(null), [devicePath]);
  if (resource.data === undefined && resource.error?.status !== 404) return resource.error
    ? html`<section class="section"><${ApiNotice} error=${resource.error} />
        <button class="button" onClick=${resource.reload}>重试</button></section>`
    : html`<${Loading} />`;
  const spec = resource.data ? displaySpec(resource.data) : null;
  const { playback, speech } = speakerControls(spec);
  return html`
    <section class="section"><h2>播报</h2><div class="card">
      <${Say} path=${speech ? devicePath : path} speech=${speech} online=${speaker.online} />
    </div></section>
    ${playback ? html`<${DeviceControls} path=${devicePath} spec=${playback}
        device=${{ did: speaker.miot_did, name: speaker.name, online: speaker.online }} />`
      : html`<section class="section"><h2>播放</h2><div class="card">
          <${Player} path=${path} online=${speaker.online} />
        </div></section>`}`;
}

function Say({ path, speech, online }) {
  const action = useAction();
  const [text, setText] = useState('');
  const submit = async e => {
    e.preventDefault();
    const result = await action.run(() => speech
      ? api.post(`${path}/actions`, { ...speech, in: [text] })
      : api.post(`${path}/tts`, { text }));
    if (result !== undefined) { setText(''); toast('已发送', 'success'); }
  };
  return html`<form class="inline-form" onSubmit=${submit}>
      <label class="grow">音箱要说的话
        <input class="input" required value=${text} onInput=${e => setText(e.currentTarget.value)} /></label>
      <button class="button button-primary" disabled=${!online || action.busy}>播报</button>
    </form>
    <${ApiNotice} error=${action.error} />`;
}

function Player({ path, online }) {
  const action = useAction();
  const status = useApi(`${path}/player`);
  const info = playerInfo(status.data);
  const [volume, setVolume] = useState(null);
  useEffect(() => { if (info?.volume !== undefined) setVolume(info.volume); }, [info?.volume]);
  const control = name => action.run(async () => { await api.post(`${path}/player`, { action: name }); status.reload(); });
  const saveVolume = value => action.run(() => api.put(`${path}/volume`, { volume: value }));
  if (!online) return html`<p class="muted">音箱离线，无法控制。</p>`;
  return html`
    <${ApiNotice} error=${status.error || action.error} />
    <div class="button-row">
      <button class="button" disabled=${action.busy} onClick=${() => control('play')}>播放</button>
      <button class="button" disabled=${action.busy} onClick=${() => control('pause')}>暂停</button>
      <button class="button" disabled=${action.busy} onClick=${() => control('stop')}>停止</button>
    </div>
    ${volume === null ? null : html`<label class="volume"><span>音量</span>
      <input type="range" min="0" max="100" value=${volume} disabled=${action.busy}
        onInput=${e => setVolume(Number(e.currentTarget.value))}
        onChange=${e => saveVolume(Number(e.currentTarget.value))} />
      <output>${volume}</output></label>`}`;
}

// answerText picks the spoken reply out of a record's answers.
function answerText(record) {
  return (record.answers || []).map(a => a.tts?.text).filter(Boolean).join(' ');
}

function Conversations({ path }) {
  const records = useApi(`${path}/conversations`);
  if (records.data === undefined) return records.error ? html`<${ApiNotice} error=${records.error} />` : html`<${Loading} />`;
  if (!records.data.length) return html`<${Empty}>暂无对话记录<//>`;
  return html`<div class="card">${records.data.map(r => html`<div key=${r.requestId} class="conversation">
    <div class="conversation-time">${dateTime(r.time)}</div>
    <div class="conversation-query">${r.query}</div>
    <div class="conversation-answer">${answerText(r)}</div>
  </div>`)}</div>`;
}
