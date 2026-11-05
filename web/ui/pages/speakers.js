import { html } from '../lib/html.js';
import { Page, Loading, Empty } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useCached, ApiNotice, OnlineStatus, byName } from '../mi.js';

export default function Speakers() {
  const { href } = usePanel();
  const speakers = useCached('/speakers');
  const list = [...(speakers.data || [])].sort(byName);
  return html`<${Page} title="音箱" subtitle="小爱音箱：播报、音量、播放控制和对话记录"
      actions=${html`<button class="button" disabled=${speakers.loading} onClick=${speakers.refresh}>刷新</button>`}>
    <${ApiNotice} error=${speakers.error} />
    ${speakers.data === undefined ? (speakers.error ? null : html`<${Loading} />`)
      : !list.length ? html`<${Empty}>暂无音箱<//>`
      : html`<div class="device-grid">${list.map(s => html`<div key=${s.id} class="card device-card">
        <div>
          <a class="cell-main truncate device-link" href=${href(`/speakers/${encodeURIComponent(s.id)}`)}>${s.name}</a>
          <div class="cell-sub truncate">${s.hardware}</div>
        </div>
        <div><${OnlineStatus} online=${s.online} /></div>
      </div>`)}</div>`}
  <//>`;
}
