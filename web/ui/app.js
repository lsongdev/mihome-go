// Mi Home panel: top navigation and the page for the location hash.
import { html, render } from './lib/html.js';
import { Navbar, DialogHost, Toaster } from './lib/ui.js';
import { createRoutes, href, useLocation } from './lib/router.js';

const nav = [
  { path: '/', title: '设备' },
  { path: '/speakers', title: '音箱' },
  { path: '/events', title: '事件' },
];

const Routes = createRoutes([
  { path: '/', load: () => import('./pages/devices.js') },
  { path: '/devices/:did', load: () => import('./pages/device.js') },
  { path: '/speakers', load: () => import('./pages/speakers.js') },
  { path: '/speakers/:id', load: () => import('./pages/speaker.js') },
  { path: '/events', load: () => import('./pages/events.js') },
]);

function App() {
  const { path, query } = useLocation();
  // The longest matching entry is active; "/" only matches exactly.
  const active = nav
    .filter(i => path === i.path || (i.path !== '/' && path.startsWith(i.path + '/')))
    .sort((a, b) => b.path.length - a.path.length)[0];
  return html`
    <${Navbar} className="topbar" brand="Mi Home" mark="◆" label="主导航"
      items=${nav.map(i => ({ href: href(i.path), title: i.title, active: i === active }))} />
    <main class="content"><${Routes} path=${path} query=${query} /></main>
    <${DialogHost} />
    <${Toaster} />`;
}

render(html`<${App} />`, document.getElementById('app'));
