// HTTP client for a module's API. The shell creates one per module with the
// module's base path, so module code only ever uses relative paths.


export class ApiError extends Error {
  constructor(status, { code = 'error', message = 'Request failed', fields } = {}) {
    super(message);
    this.status = status;
    this.code = code;
    this.fields = fields || {};
  }
}

// createClient returns a client for the API at base (relative to the page).
// With envelope (the default) successful responses are {"data": …} and that
// member is returned; without it the whole body is the result. Errors may be
// {"error": {"code","message","fields"}} or {"error": "message"}.
export function createClient(base, { envelope = true } = {}) {
  const root = new URL(base, document.baseURI);

  const url = (path, query) => {
    const u = new URL(String(path).replace(/^\//, ''), root);
    for (const [key, value] of Object.entries(query || {})) {
      if (value !== undefined && value !== null && value !== '' && value !== false) u.searchParams.set(key, value);
    }
    return u;
  };

  // request returns the result, or {data, etag, response} when raw is set.
  async function request(method, path, { body, query, signal, headers = {}, raw = false } = {}) {
    const init = { method, signal, headers: { Accept: 'application/json', ...headers } };
    if (body !== undefined) {
      init.headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(body);
    }
    let response;
    try {
      response = await fetch(url(path, query), init);
    } catch (err) {
      if (err.name === 'AbortError') throw err;
      throw new ApiError(0, { code: 'network', message: 'Cannot reach the server' });
    }
    if (response.status === 401) dispatchEvent(new Event('panel:unauthorized'));
    const payload = response.status === 204 ? null : await response.json().catch(() => null);
    if (!response.ok) {
      const error = payload?.error;
      throw new ApiError(response.status, typeof error === 'string' ? { message: error } : error || { message: response.statusText });
    }
    const data = envelope ? payload?.data ?? null : payload;
    return raw ? { data, etag: response.headers.get('ETag'), response } : data;
  }

  // stream subscribes to a server-sent event endpoint; handlers maps event
  // names to callbacks. Returns a function that closes the stream. Streams
  // that end on their own ("end", "done") are closed, not reconnected.
  function stream(path, query, handlers) {
    const source = new EventSource(url(path, query));
    for (const [event, fn] of Object.entries(handlers)) {
      if (event === 'onerror') continue;
      source.addEventListener(event, e => {
        const data = e.data ? JSON.parse(e.data) : null;
        if (event === 'end' || event === 'done') source.close();
        fn(data);
      });
    }
    source.addEventListener('error', e => {
      // A server "error" event carries data; a transport error does not.
      if (e.data) {
        source.close();
        handlers.error?.(JSON.parse(e.data));
      } else if (source.readyState === EventSource.CLOSED) {
        handlers.onerror?.();
      }
    });
    return () => source.close();
  }

  // socketURL returns a ws:// or wss:// URL for a WebSocket endpoint.
  const socketURL = (path, query) => {
    const u = url(path, query);
    u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:';
    return u;
  };

  return {
    url, request, stream, socketURL,
    get: (path, opts) => request('GET', path, opts),
    post: (path, body, opts) => request('POST', path, { ...opts, body }),
    put: (path, body, opts) => request('PUT', path, { ...opts, body }),
    patch: (path, body, opts) => request('PATCH', path, { ...opts, body }),
    del: (path, opts) => request('DELETE', path, opts),
  };
}
