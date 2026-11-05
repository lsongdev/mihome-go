import { html, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Empty, Status, toast } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, useCached, useEvents, ApiNotice, dateTime } from '../mi.js';

const LIMIT = 500;

const TITLES = {
  'device.property_changed': '属性变化',
  'device.event': '设备事件',
  'device.availability_changed': '在线状态',
  'stream.connected': '已连接',
  'stream.disconnected': '连接断开',
};

export default function Events() {
  const status = useApi('/events/status');
  const homes = useCached('/homes');
  const [events, setEvents] = useState([]);
  // Replay the server's recent history, then follow live.
  const live = useEvents(event => {
    setEvents(list => (list.some(e => e.seq === event.seq) ? list : [event, ...list].slice(0, LIMIT)));
    if (event.type.startsWith('stream.')) status.reload();
  }, { after: 0 });
  const names = new Map((homes.data || []).flatMap(home => home.devices).map(d => [d.did, d.name]));
  const s = status.data;
  return html`<${Page} title="事件" subtitle=${s ? html`<${StreamStatus} status=${s} live=${live} />` : ''}>
    <${ApiNotice} error=${status.error} />
    ${s && !s.enabled ? html`<${Notice} tone="info">实时事件未开启：在 config.yaml 中设置 events.oauth_client_id 后重启。<//>` : null}
    ${s?.enabled && !s.authorized ? html`<${Login} reload=${status.reload} />` : null}
    ${s?.error ? html`<${Notice} tone="warn">${s.error}<//>` : null}
    ${events.length ? html`<div class="card">${events.map(e => html`<${Row} key=${e.seq} event=${e} name=${names.get(e.device_id)} />`)}</div>`
      : s?.authorized ? html`<${Empty}>还没有事件。操作一下设备试试。<//>` : null}
  <//>`;
}

function StreamStatus({ status, live }) {
  if (!status.enabled || !status.authorized) return html`<${Status} state="off" label="未连接" tone="gray" />`;
  const up = status.connected && live;
  return html`<${Status} state=${up ? 'live' : 'off'} label=${up ? `实时 · ${status.devices} 台设备` : '重连中'} tone=${up ? 'green' : 'amber'} />`;
}

function Login({ reload }) {
  const { api } = usePanel();
  const action = useAction();
  const [address, setAddress] = useState('');
  const open = () => action.run(async () => {
    const { url } = await api.get('/oauth');
    window.open(url, '_blank', 'noopener,noreferrer');
  });
  const submit = async e => {
    e.preventDefault();
    const result = await action.run(() => api.post('/oauth', { url: address }));
    if (result) { setAddress(''); toast('登录成功', 'success'); reload(); }
  };
  return html`<section class="section">
    <h2>连接实时事件</h2>
    <div class="card">
      <p>实时事件需要单独登录一次小米账号。登录后浏览器会跳到 homeassistant.local，页面打不开是正常的，把地址栏的完整地址粘贴到下面。</p>
      <div class="button-row"><button class="button button-primary" disabled=${action.busy} onClick=${open}>打开小米登录</button></div>
      <form class="inline-form login-form" onSubmit=${submit}>
        <label class="grow">跳转后的地址
          <input class="input" required value=${address} placeholder="http://homeassistant.local:8123/api/webhook/...?code=..."
            onInput=${e => setAddress(e.currentTarget.value)} /></label>
        <button class="button" disabled=${action.busy}>完成登录</button>
      </form>
      <${ApiNotice} error=${action.error} />
    </div>
  </section>`;
}

function describe(event) {
  const d = event.data || {};
  switch (event.type) {
    case 'device.property_changed': return `${d.siid}.${d.piid} = ${JSON.stringify(d.value)}`;
    case 'device.event': return `${d.siid}.${d.eiid} ${JSON.stringify(d.arguments)}`;
    case 'device.availability_changed': return d.online ? '上线' : '离线';
    default: return '';
  }
}

function Row({ event, name }) {
  return html`<div class="event-row">
    <span class="event-time">${dateTime(event.time)}</span>
    <span class="event-type">${TITLES[event.type] || event.type}</span>
    <span class="event-device truncate" title=${event.device_id}>${name || event.device_id || ''}</span>
    <code class="event-value">${describe(event)}</code>
  </div>`;
}
