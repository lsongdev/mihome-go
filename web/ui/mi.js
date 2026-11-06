// Mi Home UI conventions on top of the shared components.
import { html, useEffect, useRef, useState, useResource, Alert, Badge } from '@lsongdev/scripts/react/index.js';
import { createClient } from './lib/client.js';

// api is the JSON API client; the API answers plain JSON, not {"data": …}.
export const api = createClient('api/', { envelope: false });

// useApi loads a GET endpoint.
export function useApi(path) {
  return useResource(signal => api.get(path, { signal }), [path]);
}

// useCached loads an endpoint the server caches; refresh bypasses that cache.
export function useCached(path) {
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

// useEvents calls onEvent for each realtime event, from now on or, with after, from
// that sequence number. EventSource reconnects on its own and resumes from the last
// event it saw. It returns whether the stream to this server is up.
export function useEvents(onEvent, { after } = {}) {
  const handler = useRef(onEvent);
  handler.current = onEvent;
  const [live, setLive] = useState(false);
  useEffect(() => {
    const source = new EventSource(api.url('/events', { after }));
    source.onopen = () => setLive(true);
    source.onerror = () => setLive(false);
    source.onmessage = message => handler.current(JSON.parse(message.data));
    return () => source.close();
  }, [after]);
  return live;
}

// All device views use the MIoT DID for live availability, including speakers
// whose MiNA ID belongs to a different namespace.
export function useAvailability(refresh) {
  const [online, setOnline] = useState({});
  useEvents(event => {
    if (event.type === 'device.availability_changed') {
      setOnline(previous => ({ ...previous, [event.device_id]: event.data.online }));
    }
    if (event.type === 'stream.connected') {
      setOnline({});
      refresh();
    }
  });
  return (did, fallback) => did in online ? online[did] : fallback;
}

// ApiNotice shows an API error. When Xiaomi asks for a security verification it
// links to the page that does it, since nothing works until then.
export function ApiNotice({ error }) {
  if (!error) return null;
  const url = error.fields?.url;
  if (error.code === 'verification_required' && url?.startsWith('https://')) {
    return html`<${Alert} variant="warning">
      小米账号需要安全验证。<a href=${url} target="_blank" rel="noreferrer">打开验证页面</a>，完成后刷新本页。
    <//>`;
  }
  return html`<${Alert} error=${error} />`;
}

export function OnlineStatus({ online }) {
  return html`<${Badge} variant=${online ? 'success' : undefined} dot>${online ? '在线' : '离线'}<//>`;
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
