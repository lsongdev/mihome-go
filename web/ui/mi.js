// MiService UI conventions on top of the shared components.
import { html, useRef } from './lib/html.js';
import { useResource } from './lib/hooks.js';
import { usePanel } from './lib/panel.js';
import { Notice, Status } from './lib/ui.js';

// useApi loads a GET endpoint.
export function useApi(path) {
  const { api } = usePanel();
  return useResource(signal => api.get(path, { signal }), [path]);
}

// useCached loads an endpoint the server caches; refresh bypasses that cache.
export function useCached(path) {
  const { api } = usePanel();
  const bypass = useRef(false);
  const resource = useResource(signal => {
    const query = bypass.current ? { refresh: 1 } : undefined;
    bypass.current = false;
    return api.get(path, { signal, query });
  }, [path]);
  const refresh = () => {
    bypass.current = true;
    resource.reload();
  };
  return { ...resource, refresh };
}

// ApiNotice shows an API error. When Xiaomi asks for a security verification it
// links to the page that does it, since nothing works until then.
export function ApiNotice({ error }) {
  if (!error) return null;
  const url = error.fields?.url;
  if (error.code === 'verification_required' && url?.startsWith('https://')) {
    return html`<${Notice} tone="warn">
      小米账号需要安全验证。<a href=${url} target="_blank" rel="noreferrer">打开验证页面</a>，完成后刷新本页。
    <//>`;
  }
  return html`<${Notice} error=${error} />`;
}

export function OnlineStatus({ online }) {
  return html`<${Status} state=${online ? 'online' : 'offline'} label=${online ? '在线' : '离线'} tone=${online ? 'green' : 'gray'} />`;
}

export const byName = (a, b) => a.name.localeCompare(b.name, 'zh-CN');

export const dateTime = ms => new Date(ms).toLocaleString('zh-CN', { hour12: false });

// The player status holds its details as a JSON string in data.info.
export function playerInfo(status) {
  try {
    return JSON.parse(status?.data?.info) || null;
  } catch {
    return null;
  }
}
