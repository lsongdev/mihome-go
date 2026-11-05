// UI building blocks for the pages: the @lsongdev/scripts components (in
// @lsongdev/scripts), under the names and tone vocabulary the pages use.
import { html, useLayoutEffect } from './html.js';
import {
  Alert, Badge as ScriptsBadge, Dialog, Loading, Empty, KeyValue, toast as show,
} from '../node_modules/@lsongdev/scripts/react/components/index.js';

export {
  Loading, Empty, Field, Check, ListInput, Tabs, LogView, CopyButton, ConfirmButton,
  Toaster, openDialog, DialogHost, Table,
} from '../node_modules/@lsongdev/scripts/react/components/index.js';
export { KeyValue as KV };

// Page-level navigation has its own slot, separate from metadata/actions.
export function Page({ title, subtitle, actions, back, children }) {
  return html`<section class="page">
    <header class=${back ? 'page-header page-header-with-back' : 'page-header'}>
      <div class="page-title">
        <h1>${title}</h1>
        ${subtitle ? html`<div class="page-subtitle">${subtitle}</div>` : null}
      </div>
      ${actions ? html`<div class="page-actions">${actions}</div>` : null}
      ${back ? html`<a class="page-back" href=${back.href} title=${back.label || 'Back'}><span aria-hidden="true">←</span> ${back.label || 'Back'}</a>` : null}
    </header>
    ${children}
  </section>`;
}

// Notice tones as the pages name them, mapped onto alert variants.
const variants = { error: 'danger', warn: 'warning', success: 'success', info: 'info' };

export function Notice({ error, tone = 'error', children }) {
  return html`<${Alert} variant=${variants[tone]} error=${error}>${children}<//>`;
}

// Resource renders the loading / error / empty states of a useResource result.
export function Resource({ resource, empty, children }) {
  const { data, error } = resource;
  if (data === undefined) return error ? html`<${Notice} error=${error} />` : html`<${Loading} />`;
  return html`${error ? html`<${Notice} error=${error} />` : null}
    ${Array.isArray(data) && data.length === 0 && empty ? html`<${Empty}>${empty}<//>` : children(data)}`;
}

// Badge tones as colours, mapped onto badge variants; gray is the plain badge.
const badges = { green: 'success', amber: 'warning', red: 'danger', blue: 'info' };

export function Badge({ tone, title, children }) {
  return html`<${ScriptsBadge} variant=${badges[tone]} title=${title}>${children}<//>`;
}

const tones = {
  running: 'green', healthy: 'green', succeeded: 'green', active: 'green', valid: 'green', ok: 'green',
  partial: 'amber', paused: 'amber', restarting: 'amber', starting: 'amber', waiting: 'amber', pending: 'blue',
  processing: 'blue', ready: 'blue', removing: 'amber',
  exited: 'gray', created: 'gray', stopped: 'gray', inactive: 'gray', missing: 'gray', canceled: 'gray',
  deactivated: 'gray', revoked: 'gray',
  dead: 'red', failed: 'red', invalid: 'red', unhealthy: 'red', error: 'red',
};

const capitalize = s => (s ? s[0].toUpperCase() + s.slice(1) : s);

// Status renders a state as a badge with a dot, labelled with the state
// unless the page passes a label. Pages pass a tone for states not listed
// above.
export function Status({ state, label, tone, title }) {
  return html`<${ScriptsBadge} variant=${badges[tone || tones[state]]} dot title=${title}>${label || capitalize(state)}<//>`;
}

// Modal is a dialog shown for as long as it is rendered; onClose asks the
// owner to stop rendering it.
export function Modal({ title, onClose, children, footer, wide, closeOnBackdrop = false }) {
  useLayoutEffect(() => {
    // document.activeElement is the shadow host for mounted modules.
    let previous = document.activeElement;
    while (previous?.shadowRoot?.activeElement) previous = previous.shadowRoot.activeElement;
    return () => queueMicrotask(() => { if (previous?.isConnected) previous.focus(); });
  }, []);
  // Native close controls fire after closing. Reopen if the owner vetoes
  // dismissal (unsaved input or a pending request). Escape can be stopped
  // before the native dialog closes at all.
  const close = event => {
    if (onClose?.() === false && event.currentTarget.isConnected) event.currentTarget.showModal();
  };
  const cancel = event => { event.preventDefault(); onClose?.(); };
  return html`<${Dialog} open title=${title} footer=${footer} onClose=${close} onCancel=${cancel}
    closeOnBackdrop=${closeOnBackdrop} className=${wide ? 'dialog-wide' : ''}>${children}<//>`;
}

// toast shows a message; tone is info, success or error.
export function toast(message, tone = 'info') {
  show(message, tone === 'error' ? 'danger' : tone);
}

export function toastError(err) {
  toast(err?.message || String(err), 'error');
}

// confirmDanger asks before destructive actions.
export function confirmDanger(message) {
  return window.confirm(message);
}
