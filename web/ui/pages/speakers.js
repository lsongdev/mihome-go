import { html, PageHeader, Loading, Empty, useRouter } from '@lsongdev/scripts/react/index.js';
import { useCached, useAvailability, ApiNotice, OnlineStatus, byName } from '../mi.js';

export default function Speakers() {
  const router = useRouter();
  const speakers = useCached('/speakers');
  const online = useAvailability(speakers.refresh);
  const list = (speakers.data || []).map(s => ({ ...s, online: online(s.miot_did, s.online) })).sort(byName);
  return html`<${PageHeader} title="音箱" subtitle="小爱音箱：播报、音量、播放控制和对话记录"
      actions=${html`<button class="button" disabled=${speakers.loading} onClick=${speakers.refresh}>刷新</button>`} />
    <${ApiNotice} error=${speakers.error} />
    ${speakers.data === undefined ? (speakers.error ? null : html`<${Loading} />`)
      : !list.length ? html`<${Empty}>暂无音箱<//>`
      : html`<div class="device-grid">${list.map(s => html`<div key=${s.id} class="card device-card">
        <div>
          <a class="text-truncate device-link" href=${router.href(`/speakers/${encodeURIComponent(s.id)}`)}>${s.name}</a>
          <div class="muted text-sm text-truncate">${s.hardware}</div>
        </div>
        <div><${OnlineStatus} online=${s.online} /></div>
      </div>`)}</div>`}`;
}
