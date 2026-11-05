// Runs the module on its own: a minimal host that reads manifest.json for
// the top navigation and drives mount() from the URL hash, exactly as a
// panel shell would.
import { html, render } from './lib/html.js';
import { Navbar } from './node_modules/@lsongdev/scripts/react/components/navbar.js';
import { mount } from './module.js';
import { createNavigation } from './lib/navigation.js';

const navigation = createNavigation();

const manifest = await (await fetch('manifest.json')).json();
document.title = manifest.title;

function location() {
  const raw = window.location.hash.replace(/^#/, '') || '/';
  const [path, search = ''] = raw.split('?');
  return { path: path || '/', query: Object.fromEntries(new URLSearchParams(search)) };
}

const href = (path, query) => {
  const s = new URLSearchParams(Object.entries(query || {}).filter(([, v]) => v !== '' && v != null)).toString();
  return '#' + path + (s ? '?' + s : '');
};

const navigate = navigation.navigate;

function context() {
  return {
    id: manifest.id,
    apiBase: new URL('api/', document.baseURI).href,
    assetsBase: new URL('./', document.baseURI).href,
    ...location(),
    navigate, href,
    beforeLeave: navigation.beforeLeave,
    link: () => null, // no other modules standalone
    lang: document.documentElement.lang,
  };
}

function TopBar({ path }) {
  // The longest matching entry is active; "/" only matches exactly.
  const active = (manifest.nav || [])
    .filter(i => path === i.path || (i.path !== '/' && path.startsWith(i.path + '/')))
    .sort((a, b) => b.path.length - a.path.length)[0];
  return html`
    <${Navbar} className="topbar" brand=${manifest.title} mark="◆" label="主导航"
      items=${(manifest.nav || []).map(i => ({ href: href(i.path), title: i.title, active: i === active }))} />`;
}

const topbar = document.getElementById('topbar');
const instance = mount(document.getElementById('module'), context());
const update = () => {
  const ctx = context();
  // The module root opens its home page.
  if (ctx.path === '/' && manifest.home && manifest.home !== '/') return navigate(manifest.home, { replace: true });
  render(html`<${TopBar} path=${ctx.path} />`, topbar);
  instance.update(ctx);
};
addEventListener('panel:routechange', update);
update();
