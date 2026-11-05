// Device controls generated from a MIoT specification: each property becomes a
// control according to its access, format, value list and range, and each action
// a button or a small form.
import { html, useEffect, useRef, useState } from './lib/html.js';
import { useResource } from './lib/hooks.js';
import { usePanel } from './lib/panel.js';
import { Notice, Empty, toast, toastError, confirmDanger } from './lib/ui.js';
import { ApiNotice, useEvents } from './mi.js';

// Writing to these is confirmed first: it can open a door or start heating.
// Actions are always confirmed, since a spec does not say what they do.
const RISKY_TYPES = new Set(['lock', 'microwave-oven', 'oven', 'cooker', 'induction-cooker', 'gas-stove', 'heater', 'water-heater']);

const UNITS = {
  percentage: '%', celsius: '°C', fahrenheit: '°F', kelvin: 'K', seconds: '秒', minutes: '分钟', hours: '小时', days: '天',
  watt: 'W', kWh: 'kWh', lux: 'lx', pascal: 'Pa', arcdegrees: '°',
};
const unitText = unit => UNITS[unit] ?? unit ?? '';

const readable = p => p.access.includes('read');
const writable = p => p.access.includes('write');
const isNumber = p => /^(u?int\d+|float|double)$/.test(p.format);

// Device information is the least interesting service: it goes last.
const ordered = spec => [...spec.services].sort((a, b) =>
  (a.type === 'device-information') - (b.type === 'device-information'));

function formatValue(p, entry) {
  // Some properties can't be read: they only arrive in notifications or as event arguments.
  if (!entry) return readable(p) ? '—' : html`<span class="muted">不可读取</span>`;
  if (entry.code) return html`<span class="muted" title=${`错误码 ${entry.code}`}>不可用</span>`;
  const { value } = entry;
  const option = p.options?.find(o => o.value === value);
  if (option) return option.name;
  if (typeof value === 'boolean') return value ? '是' : '否';
  if (value === null || value === undefined || value === '') return '—';
  if (typeof value === 'object') return JSON.stringify(value);
  const unit = unitText(p.unit);
  return unit ? `${value} ${unit}` : String(value);
}

// useValues reads every readable property of a device. Each result has its own
// code, because a device can fail on some properties only.
function useValues(path, spec, online) {
  const { api } = usePanel();
  const iids = spec.services.flatMap(s => s.properties.filter(readable).map(p => `${s.iid}.${p.iid}`));
  const query = iids.map(iid => `iid=${iid}`).join('&');
  const resource = useResource(signal => (online && iids.length
    ? api.get(`${path}/props?${query}`, { signal }) : Promise.resolve([])), [path, query, online]);
  const [values, setValues] = useState({});
  useEffect(() => {
    if (!resource.data) return;
    setValues(Object.fromEntries(resource.data.map(r => [`${r.siid}.${r.piid}`, { value: r.value, code: r.code }])));
  }, [resource.data]);
  return { ...resource, values, setValues };
}

export function DeviceControls({ path, spec, device }) {
  const { api } = usePanel();
  const values = useValues(path, spec, device.online);
  // Devices apply a write a moment later, so read back after a delay.
  const timer = useRef(0);
  useEffect(() => () => clearTimeout(timer.current), []);
  const readBack = () => {
    clearTimeout(timer.current);
    timer.current = setTimeout(values.reload, 1500);
  };
  const risky = RISKY_TYPES.has(spec.type);
  useEvents(event => {
    // Changes during an outage were not delivered: read everything again.
    if (event.type === 'stream.connected') values.reload();
    if (event.type === 'device.property_changed' && event.device_id === device.did) {
      const { siid, piid, value } = event.data;
      values.setValues(v => ({ ...v, [`${siid}.${piid}`]: { value } }));
    }
  });

  const write = async (service, property, value) => {
    if (risky && !confirmDanger(`确定要修改「${device.name}」的「${property.name}」吗？`)) return false;
    try {
      await api.put(`${path}/props`, { siid: service.iid, piid: property.iid, value });
    } catch (error) {
      toastError(error);
      return false;
    }
    values.setValues(v => ({ ...v, [`${service.iid}.${property.iid}`]: { value } }));
    readBack();
    return true;
  };
  const run = async (service, action, args) => {
    if (!confirmDanger(`确定要在「${device.name}」上执行「${action.name}」吗？`)) return;
    try {
      const result = await api.post(`${path}/actions`, { siid: service.iid, aiid: action.iid, in: args });
      toast(result?.out?.length ? `${action.name}：${JSON.stringify(result.out)}` : `${action.name}：已发送`, 'success');
      readBack();
    } catch (error) {
      toastError(error);
    }
  };

  const services = ordered(spec);
  if (!services.length) return html`<${Empty}>这台设备的规格里没有服务<//>`;
  return html`
    ${device.online ? null : html`<${Notice} tone="warn">设备离线，无法读取状态或控制。<//>`}
    <${ApiNotice} error=${values.error} />
    <div class="page-actions control-actions">
      <button class="button button-small" disabled=${!device.online || values.loading} onClick=${values.reload}>刷新状态</button>
    </div>
    ${services.map(service => html`<${Service} key=${service.iid} service=${service} values=${values.values}
      disabled=${!device.online} write=${write} run=${run} />`)}`;
}

function Service({ service, values, disabled, write, run }) {
  const { properties, actions, events } = service;
  return html`<section class="section">
    <h2>${service.name}</h2>
    <div class="card controls">
      ${properties.map(p => html`<${Property} key=${p.iid} property=${p} entry=${values[`${service.iid}.${p.iid}`]}
        disabled=${disabled} write=${value => write(service, p, value)} />`)}
      ${actions.map(a => html`<${Action} key=${a.iid} service=${service} action=${a}
        disabled=${disabled} run=${args => run(service, a, args)} />`)}
      ${events.map(e => html`<${SpecEvent} key=${e.iid} service=${service} event=${e} />`)}
      ${properties.length + actions.length + events.length ? null : html`<p class="muted">这个服务没有属性、动作或事件。</p>`}
    </div>
  </section>`;
}

function Property({ property: p, entry, disabled, write }) {
  // Remounting the input restores the real value when a write is refused.
  const [attempt, setAttempt] = useState(0);
  const commit = async value => { if (!await write(value)) setAttempt(n => n + 1); };
  const value = entry && !entry.code ? entry.value : undefined;
  const wide = writable(p) && p.format !== 'bool' && !p.options?.length;
  return html`<div class=${wide ? 'control control-wide' : 'control'}>
    <span class="control-label">${p.name}</span>
    ${writable(p)
      ? html`<${Input} key=${attempt} property=${p} value=${value} disabled=${disabled} commit=${commit} />`
      : html`<span class="control-value">${formatValue(p, entry)}</span>`}
  </div>`;
}

// Input picks the control for a writable property.
function Input({ property: p, value, disabled, commit }) {
  if (p.format === 'bool') {
    return html`<span class="control-end"><label class="toggle" title=${p.name}>
      <input type="checkbox" aria-label=${p.name} checked=${value === true} disabled=${disabled}
        onChange=${e => commit(e.currentTarget.checked)} /><span></span>
    </label></span>`;
  }
  if (p.options?.length) return html`<${Select} property=${p} value=${value} disabled=${disabled} onPick=${commit} />`;
  if (p.range) return html`<${Range} property=${p} value=${value} disabled=${disabled} commit=${commit} />`;
  return html`<${Field} property=${p} value=${value} disabled=${disabled} commit=${commit} />`;
}

function Select({ property: p, value, disabled, onPick }) {
  const known = p.options.some(o => o.value === value);
  return html`<select class="input" aria-label=${p.name} disabled=${disabled} value=${known ? String(value) : ''}
      onChange=${e => onPick(p.options.find(o => String(o.value) === e.currentTarget.value).value)}>
    ${known ? null : html`<option value="" disabled>${value === undefined ? '—' : `未知值 ${value}`}</option>`}
    ${p.options.map(o => html`<option key=${String(o.value)} value=${String(o.value)}>${o.name}</option>`)}
  </select>`;
}

function Range({ property: p, value, disabled, commit }) {
  const [draft, setDraft] = useState(value ?? p.range.min);
  useEffect(() => { if (value !== undefined) setDraft(value); }, [value]);
  return html`<span class="control-range">
    <input class="range" type="range" aria-label=${p.name} min=${p.range.min} max=${p.range.max} step=${p.range.step}
      value=${draft} disabled=${disabled || value === undefined}
      onInput=${e => setDraft(Number(e.currentTarget.value))}
      onChange=${e => commit(Number(e.currentTarget.value))} />
    <output>${value === undefined ? '—' : `${draft}${unitText(p.unit) ? ' ' + unitText(p.unit) : ''}`}</output>
  </span>`;
}

function Field({ property: p, value, disabled, commit }) {
  const [draft, setDraft] = useState(value ?? '');
  useEffect(() => { if (value !== undefined) setDraft(value); }, [value]);
  const submit = e => {
    e.preventDefault();
    commit(isNumber(p) ? Number(draft) : draft);
  };
  return html`<form class="control-field" onSubmit=${submit}>
    <input class="input" aria-label=${p.name} type=${isNumber(p) ? 'number' : 'text'} step="any" required disabled=${disabled}
      value=${draft} onInput=${e => setDraft(e.currentTarget.value)} />
    <button class="button button-small" disabled=${disabled}>设置</button>
  </form>`;
}

const initialArgument = p => {
  if (p.format === 'bool') return false;
  if (p.options?.length) return p.options[0].value;
  if (p.range) return p.range.min;
  return isNumber(p) ? 0 : '';
};

// An iid that the spec does not describe is still listed, by number.
const propertyNames = (service, iids) => iids
  .map(iid => service.properties.find(p => p.iid === iid)?.name ?? `#${iid}`).join('、');

// SpecEvent lists something the device can report; the 事件 page shows them as they happen.
function SpecEvent({ service, event }) {
  return html`<div class="control">
    <span class="control-label">${event.name}</span>
    <span class="control-value muted">事件${event.arguments.length ? `，参数：${propertyNames(service, event.arguments)}` : ''}</span>
  </div>`;
}

function Action({ service, action, disabled, run }) {
  const params = action.in.map(iid => service.properties.find(p => p.iid === iid)).filter(Boolean);
  const [args, setArgs] = useState(() => params.map(initialArgument));
  const [busy, setBusy] = useState(false);
  const set = (i, value) => setArgs(list => list.map((old, n) => (n === i ? value : old)));
  // Arguments that the spec does not describe cannot be filled in here.
  const unknown = params.length !== action.in.length;
  const submit = async e => {
    e.preventDefault();
    setBusy(true);
    try { await run(args); } finally { setBusy(false); }
  };
  const details = params.length > 0 || action.out.length > 0;
  return html`<form class="control control-action" onSubmit=${submit}>
    <span class="control-label">${action.name}</span>
    ${details ? html`<span class="control-args">
      ${params.map((p, i) => html`<${Argument} key=${p.iid} property=${p} value=${args[i]} set=${v => set(i, v)} />`)}
      ${action.out.length ? html`<span class="muted">返回：${propertyNames(service, action.out)}</span>` : null}
    </span>` : null}
    <button class="button button-small" disabled=${disabled || busy || unknown}
      title=${unknown ? '规格中缺少参数定义，请用高级调试' : undefined}>执行</button>
  </form>`;
}

function Argument({ property: p, value, set }) {
  if (p.format === 'bool') {
    return html`<label class="control-arg"><input type="checkbox" checked=${value} onChange=${e => set(e.currentTarget.checked)} /> ${p.name}</label>`;
  }
  if (p.options?.length) return html`<${Select} property=${p} value=${value} onPick=${set} />`;
  const number = isNumber(p);
  return html`<input class="input" aria-label=${p.name} placeholder=${p.name} required=${!number}
    type=${number ? 'number' : 'text'} step=${p.range?.step ?? 'any'} min=${p.range?.min} max=${p.range?.max} value=${value}
    onInput=${e => set(number ? Number(e.currentTarget.value) : e.currentTarget.value)} />`;
}
