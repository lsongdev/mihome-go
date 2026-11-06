// The Mi Home panel: navigation and the page for the location hash.
import { html, render, lazy, useRoute, RouterView, Navbar, Alert, DialogHost, Toaster } from '@lsongdev/scripts/react/index.js';
import { createRouter } from '@lsongdev/scripts/navigation/router.js';

const router = createRouter([
  ['/', lazy(() => import('./pages/devices.js'))],
  ['/devices/:did', lazy(() => import('./pages/device.js'))],
  ['/speakers', lazy(() => import('./pages/speakers.js'))],
  ['/speakers/:id', lazy(() => import('./pages/speaker.js'))],
  ['/events', lazy(() => import('./pages/events.js'))],
], { hash: true });
router.start();

const nav = [
  { href: '#/', title: '设备' },
  { href: '#/speakers', title: '音箱' },
  { href: '#/events', title: '事件' },
];

function App() {
  const route = useRoute(router);
  return html`
    <${Navbar} className="container" brand="Mi Home" mark="◆" label="主导航" items=${nav} current=${router.href(route.path)} />
    <main class="container stack">
      <${RouterView} router=${router} notFound=${html`<${Alert} variant="warning">页面不存在<//>`} />
    </main>
    <${DialogHost} />
    <${Toaster} />`;
}

render(html`<${App} />`, document.getElementById('app'));
