import { html, useState } from '../lib/html.js';
import { Page, Loading, Empty, Badge } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useCached, useEvents, ApiNotice, OnlineStatus, byName } from '../mi.js';

const FILTERS = [['', '全部'], ['online', '在线'], ['offline', '离线']];

export default function Devices() {
  const { query, setQuery } = usePanel();
  const homes = useCached('/homes');
  // Availability changes since the list was loaded.
  const [online, setOnline] = useState({});
  useEvents(event => {
    if (event.type === 'device.availability_changed') setOnline(o => ({ ...o, [event.device_id]: event.data.online }));
    if (event.type === 'stream.connected') { setOnline({}); homes.refresh(); }
  });
  const current = d => (d.did in online ? { ...d, online: online[d.did] } : d);
  const search = (query.q || '').toLowerCase();
  const matches = d => (!query.status || d.online === (query.status === 'online'))
    && (!search || [d.name, d.model, d.did].join(' ').toLowerCase().includes(search));
  const shown = (homes.data || [])
    .map(home => ({ ...home, devices: home.devices.map(current).filter(matches).sort(byName) }))
    .filter(home => home.devices.length);
  const all = (homes.data || []).flatMap(home => home.devices.map(current));

  return html`<${Page} title="设备"
      subtitle=${homes.data ? `${homes.data.length} 个家庭 · ${all.length} 台设备 · ${all.filter(d => d.online).length} 台在线` : ''}
      actions=${html`<button class="button" disabled=${homes.loading} onClick=${homes.refresh}>刷新</button>`}>
    <${ApiNotice} error=${homes.error} />
    <div class="toolbar">
      <input class="input" type="search" placeholder="搜索名称、型号或 DID" aria-label="搜索设备" value=${query.q || ''}
        onInput=${e => setQuery({ q: e.currentTarget.value })} />
      <div class="segmented">${FILTERS.map(([value, text]) => html`<button key=${value}
        class=${`button ${(query.status || '') === value ? 'active' : ''}`} onClick=${() => setQuery({ status: value })}>${text}</button>`)}</div>
    </div>
    ${homes.data === undefined ? (homes.error ? null : html`<${Loading} />`)
      : !shown.length ? html`<${Empty}>${all.length ? '没有匹配的设备' : '暂无设备'}<//>`
      : shown.map(home => html`<${HomeSection} key=${home.id} home=${home} />`)}
  <//>`;
}

function HomeSection({ home }) {
  const { href } = usePanel();
  return html`<section class="section">
    <h2 class="home-head">${home.name} ${home.shared ? html`<${Badge} tone="blue">共享<//>` : null}</h2>
    <div class="device-grid">${home.devices.map(d => html`<div key=${d.did} class="card device-card">
      <div>
        <a class="cell-main truncate device-link" href=${href(`/devices/${encodeURIComponent(d.did)}`)}>${d.name}</a>
        <div class="cell-sub truncate">${d.model}</div>
      </div>
      <div><${OnlineStatus} online=${d.online} /></div>
    </div>`)}</div>
  </section>`;
}
