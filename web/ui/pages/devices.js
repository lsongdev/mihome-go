import { html, PageHeader, Loading, Empty, Badge, useRouter } from '@lsongdev/scripts/react/index.js';
import { useCached, useAvailability, ApiNotice, OnlineStatus, byName } from '../mi.js';

const FILTERS = [['', '全部'], ['online', '在线'], ['offline', '离线']];

export default function Devices({ query }) {
  const router = useRouter();
  const homes = useCached('/homes');
  const online = useAvailability(homes.refresh);
  const current = d => ({ ...d, online: online(d.did, d.online) });
  const search = (query.q || '').toLowerCase();
  const matches = d => (!query.status || d.online === (query.status === 'online'))
    && (!search || [d.name, d.model, d.did].join(' ').toLowerCase().includes(search));
  const shown = (homes.data || [])
    .map(home => ({ ...home, devices: home.devices.map(current).filter(matches).sort(byName) }))
    .filter(home => home.devices.length);
  const all = (homes.data || []).flatMap(home => home.devices.map(current));

  return html`<${PageHeader} title="设备"
      subtitle=${homes.data ? `${homes.data.length} 个家庭 · ${all.length} 台设备 · ${all.filter(d => d.online).length} 台在线` : ''}
      actions=${html`<button class="button" disabled=${homes.loading} onClick=${homes.refresh}>刷新</button>`} />
    <${ApiNotice} error=${homes.error} />
    <div class="toolbar">
      <input class="input" type="search" placeholder="搜索名称、型号或 DID" aria-label="搜索设备" value=${query.q || ''}
        onInput=${e => router.setQuery({ q: e.currentTarget.value })} />
      <div class="button-group">${FILTERS.map(([value, text]) => html`<button key=${value}
        class=${`button ${(query.status || '') === value ? 'active' : ''}`} onClick=${() => router.setQuery({ status: value })}>${text}</button>`)}</div>
    </div>
    ${homes.data === undefined ? (homes.error ? null : html`<${Loading} />`)
      : !shown.length ? html`<${Empty}>${all.length ? '没有匹配的设备' : '暂无设备'}<//>`
      : shown.map(home => html`<${HomeSection} key=${home.id} home=${home} />`)}`;
}

function HomeSection({ home }) {
  const router = useRouter();
  return html`<section class="stack">
    <h2 class="section-title home-head">${home.name} ${home.shared ? html`<${Badge} variant="info">共享<//>` : null}</h2>
    <div class="device-grid">${home.devices.map(d => html`<div key=${d.did} class="card device-card">
      <div>
        <a class="text-truncate device-link" href=${router.href(`/devices/${encodeURIComponent(d.did)}`)}>${d.name}</a>
        <div class="muted text-sm text-truncate">${d.model}</div>
      </div>
      <div><${OnlineStatus} online=${d.online} /></div>
    </div>`)}</div>
  </section>`;
}
