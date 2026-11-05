import { html, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, KV, Badge, toast, confirmDanger } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, useCached, ApiNotice, OnlineStatus } from '../mi.js';
import { DeviceControls } from '../controls.js';
import { CameraView } from '../camera.js';

const SPEC_URL = 'https://miot-spec.org/miot-spec-v2/instances?status=all';

export default function Device() {
  const { params } = usePanel();
  const path = `/devices/${encodeURIComponent(params.did)}`;
  const resource = useCached(path);
  const spec = useApi(`${path}/spec`);
  const actions = html`<button class="button" disabled=${resource.loading} onClick=${resource.refresh}>刷新</button>`;
  if (resource.data === undefined) {
    return html`<${Page} title="设备" actions=${actions}>
      ${resource.error ? html`<${ApiNotice} error=${resource.error} />` : html`<${Loading} />`}
    <//>`;
  }
  const { device: d, home, shared } = resource.data;
  return html`<${Page} title=${d.name} subtitle=${html`<${OnlineStatus} online=${d.online} />`} actions=${actions}>
    <section class="section">
      <h2>设备信息</h2>
      <div class="card"><${KV} items=${[
        ['家庭', html`${home} ${shared ? html`<${Badge} tone="blue">共享<//>` : null}`],
        ['型号', d.model],
        ['DID', html`<code>${d.did}</code>`],
        ['规格', spec.data?.name],
      ]} /></div>
    </section>
    ${spec.data?.type === 'camera' ? html`<${CameraView} key=${path} path=${path} online=${d.online} />` : null}
    ${spec.data ? html`<${DeviceControls} key=${spec.data.urn} path=${path} spec=${spec.data} device=${d} />`
      : spec.error ? html`<section class="section"><${ApiNotice} error=${spec.error} /></section>`
      : spec.loading ? html`<${Loading} />`
      : html`<section class="section"><p class="muted">这台设备没有 MIoT 规格，只能用下面的高级调试。</p></section>`}
    <details class="section advanced" open=${spec.data === null || Boolean(spec.error)}>
      <summary>高级调试</summary>
      <p class="muted">按 <a href=${SPEC_URL} target="_blank" rel="noreferrer">MIoT 规格</a> 中的 siid / piid / aiid 操作。写属性和执行动作会真实控制设备。</p>
      <div class="card">
        <${ReadProp} path=${path} />
        <${WriteProp} path=${path} name=${d.name} />
        <${RunAction} path=${path} name=${d.name} />
      </div>
    </details>
  <//>`;
}

// parseJSON reads a typed value; anything that is not valid JSON is a string.
function parseJSON(text) {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

const Id = ({ label, value, set }) => html`<label>${label}
  <input class="input input-number" type="number" min="1" required value=${value} onInput=${e => set(e.currentTarget.value)} /></label>`;

const Result = ({ action, value }) => html`
  ${action.error ? html`<${ApiNotice} error=${action.error} />` : null}
  ${value !== undefined && !action.error ? html`<pre class="result">${JSON.stringify(value, null, 2)}</pre>` : null}`;

function ReadProp({ path }) {
  const { api } = usePanel();
  const action = useAction();
  const [siid, setSiid] = useState('2');
  const [piid, setPiid] = useState('1');
  const [value, setValue] = useState();
  const submit = async e => {
    e.preventDefault();
    const result = await action.run(() => api.get(`${path}/props`, { query: { iid: `${siid}.${piid}` } }));
    if (result !== undefined) setValue(result);
  };
  return html`<div>
    <p class="form-title">读属性</p>
    <form class="inline-form" onSubmit=${submit}>
      <${Id} label="siid" value=${siid} set=${setSiid} />
      <${Id} label="piid" value=${piid} set=${setPiid} />
      <button class="button" disabled=${action.busy}>读取</button>
    </form>
    <${Result} action=${action} value=${value} />
  </div>`;
}

function WriteProp({ path, name }) {
  const { api } = usePanel();
  const action = useAction();
  const [siid, setSiid] = useState('2');
  const [piid, setPiid] = useState('1');
  const [text, setText] = useState('');
  const [value, setValue] = useState();
  const submit = async e => {
    e.preventDefault();
    if (!confirmDanger(`确定要写入「${name}」的属性 ${siid}.${piid} 吗？`)) return;
    const body = { siid: Number(siid), piid: Number(piid), value: parseJSON(text) };
    const result = await action.run(() => api.put(`${path}/props`, body));
    if (result !== undefined) { setValue(result); toast('已发送', 'success'); }
  };
  return html`<div class="section">
    <p class="form-title">写属性</p>
    <form class="inline-form" onSubmit=${submit}>
      <${Id} label="siid" value=${siid} set=${setSiid} />
      <${Id} label="piid" value=${piid} set=${setPiid} />
      <label class="grow">值（JSON，如 true、30、"text"）
        <input class="input" required value=${text} onInput=${e => setText(e.currentTarget.value)} /></label>
      <button class="button" disabled=${action.busy}>写入</button>
    </form>
    <${Result} action=${action} value=${value} />
  </div>`;
}

function RunAction({ path, name }) {
  const { api } = usePanel();
  const action = useAction();
  const [siid, setSiid] = useState('2');
  const [aiid, setAiid] = useState('1');
  const [text, setText] = useState('[]');
  const [invalid, setInvalid] = useState(false);
  const [value, setValue] = useState();
  const submit = async e => {
    e.preventDefault();
    const args = parseJSON(text);
    setInvalid(!Array.isArray(args));
    if (!Array.isArray(args)) return;
    if (!confirmDanger(`确定要在「${name}」上执行动作 ${siid}.${aiid} 吗？`)) return;
    const result = await action.run(() => api.post(`${path}/actions`, { siid: Number(siid), aiid: Number(aiid), in: args }));
    if (result !== undefined) { setValue(result); toast('已发送', 'success'); }
  };
  return html`<div class="section">
    <p class="form-title">执行动作</p>
    <form class="inline-form" onSubmit=${submit}>
      <${Id} label="siid" value=${siid} set=${setSiid} />
      <${Id} label="aiid" value=${aiid} set=${setAiid} />
      <label class="grow">参数（JSON 数组）
        <input class="input" value=${text} onInput=${e => setText(e.currentTarget.value)} /></label>
      <button class="button" disabled=${action.busy}>执行</button>
    </form>
    ${invalid ? html`<${Notice}>参数必须是 JSON 数组<//>` : null}
    <${Result} action=${action} value=${value} />
  </div>`;
}
