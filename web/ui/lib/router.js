// Hash routes ("#/path?key=value"): the current location as a hook, links,
// navigation, and pages loaded on first use.
import { html, useEffect, useState } from './html.js';
import { Alert, Loading } from './ui.js';

const search = query => {
  const s = new URLSearchParams(Object.entries(query || {}).filter(([, v]) => v !== '' && v != null)).toString();
  return s ? '?' + s : '';
};

// current returns the path and query of the location hash.
function current() {
  const raw = window.location.hash.replace(/^#/, '') || '/';
  const [path, s = ''] = raw.split('?');
  return { path: path || '/', query: Object.fromEntries(new URLSearchParams(s)) };
}

// href returns the link to a path with a query; empty values are left out.
export const href = (path, query) => '#' + path + search(query);

// navigate goes to a path, which may include a query. Both ways fire hashchange.
export function navigate(to, { replace = false } = {}) {
  if (replace) window.location.replace('#' + to);
  else window.location.hash = to;
}

// setQuery changes some query values of the current page without adding a
// history entry, so filters can be shared as links.
export function setQuery(patch) {
  const { path, query } = current();
  navigate(path + search({ ...query, ...patch }), { replace: true });
}

// useLocation returns { path, query } and updates when the hash changes.
export function useLocation() {
  const [location, setLocation] = useState(current);
  useEffect(() => {
    const update = () => setLocation(current());
    addEventListener('hashchange', update);
    return () => removeEventListener('hashchange', update);
  }, []);
  return location;
}

// compile turns "/torrents/:hash" into a matcher that returns params.
function compile(pattern) {
  const names = [];
  const re = new RegExp('^' + pattern.replace(/:(\w+)/g, (_, name) => { names.push(name); return '([^/]+)'; }) + '/?$');
  return path => {
    const m = path.match(re);
    return m && Object.fromEntries(names.map((n, i) => [n, decodeURIComponent(m[i + 1])]));
  };
}

// createRoutes returns a component that renders the first of routes
// ([{ path, load: () => import(…) }]) matching the location. Pages get
// { params, query } as props.
export function createRoutes(routes, { notFound = '页面不存在' } = {}) {
  const table = routes.map(route => ({ ...route, match: compile(route.path) }));
  const loaded = new Map();

  function Lazy({ route, props }) {
    const [, force] = useState(0);
    const [error, setError] = useState(null);
    const mod = loaded.get(route);
    useEffect(() => {
      if (mod) return;
      route.load().then(m => { loaded.set(route, m); force(n => n + 1); }).catch(setError);
    }, [route]);
    if (error) return html`<${Alert} error=${error} />`;
    if (!mod) return html`<${Loading} />`;
    const Page = mod.default;
    return html`<${Page} ...${props} />`;
  }

  return function Routes({ path, query }) {
    for (const route of table) {
      const params = route.match(path);
      if (params) return html`<${Lazy} key=${path} route=${route} props=${{ params, query }} />`;
    }
    return html`<${Alert} variant="warning">${notFound}<//>`;
  };
}
