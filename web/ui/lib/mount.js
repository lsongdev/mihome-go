// createMount implements the module contract's mount(el, ctx) for a table of
// routes: it matches ctx.path, lazy-loads the page, and gives it the page
// context (as props and through usePanel). Dialogs and toasts render inside
// el, so they work in a shadow root too.
import { html, render, useEffect, useState } from './html.js';
import { createClient } from './client.js';
import { PanelContext } from './panel.js';
import { DialogHost, Toaster, Notice, Loading } from './ui.js';

// compile turns "/containers/:id" into a matcher that returns params.
function compile(pattern) {
  const names = [];
  const re = new RegExp('^' + pattern.replace(/:(\w+)/g, (_, name) => { names.push(name); return '([^/]+)'; }) + '/?$');
  return path => {
    const m = path.match(re);
    return m && Object.fromEntries(names.map((n, i) => [n, decodeURIComponent(m[i + 1])]));
  };
}

const search = query => {
  const s = new URLSearchParams(Object.entries(query || {}).filter(([, v]) => v !== '' && v != null)).toString();
  return s ? '?' + s : '';
};

function Lazy({ route, loaded, props }) {
  const [, force] = useState(0);
  const [error, setError] = useState(null);
  const mod = loaded.get(route);
  useEffect(() => {
    if (mod) return;
    route.load().then(m => { loaded.set(route, m); force(n => n + 1); }).catch(setError);
  }, [route]);
  if (error) return html`<${Notice} error=${error} />`;
  if (!mod) return html`<${Loading} />`;
  const Page = mod.default;
  return html`<${PanelContext.Provider} value=${props}><${Page} ...${props} /><//>`;
}

// routes: [{ path, load: () => import(...) }], matched in order.
// api(client) optionally wraps the API client in a typed API.
// envelope: whether the API answers {"data": …} (see createClient).
export function createMount({ routes, api: wrap, envelope = true }) {
  const table = routes.map(route => ({ ...route, match: compile(route.path) }));
  const loaded = new Map();

  function App({ ctx, api, client }) {
    let match = null;
    for (const route of table) {
      const params = route.match(ctx.path);
      if (params) { match = { route, params }; break; }
    }
    const props = match && {
      api, client, params: match.params, query: ctx.query,
      navigate: ctx.navigate, href: ctx.href, link: ctx.link, beforeLeave: ctx.beforeLeave,
      // Filters live in the query string so views are shareable.
      setQuery: patch => ctx.navigate(ctx.path + search({ ...ctx.query, ...patch }), { replace: true }),
    };
    return html`<div class="module-root">
      ${match ? html`<${Lazy} key=${ctx.path} route=${match.route} loaded=${loaded} props=${props} />` : html`<${Notice}>Page not found<//>`}
      <${DialogHost} />
      <${Toaster} />
    </div>`;
  }

  return function mount(el, ctx) {
    const client = createClient(ctx.apiBase, { envelope });
    const api = wrap ? wrap(client) : client;
    const draw = c => render(html`<${App} ctx=${c} api=${api} client=${client} />`, el);
    draw(ctx);
    return {
      update(next) { draw(next); },
      unmount() { render(null, el); },
    };
  };
}
