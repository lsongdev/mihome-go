// Mi Home panel entry point (panel module contract, version 1). The host is a
// panel shell, or standalone.js on its own.
import { createMount } from './lib/mount.js';

const routes = [
  { path: '/', load: () => import('./pages/devices.js') },
  { path: '/devices/:did', load: () => import('./pages/device.js') },
  { path: '/speakers', load: () => import('./pages/speakers.js') },
  { path: '/speakers/:id', load: () => import('./pages/speaker.js') },
  { path: '/events', load: () => import('./pages/events.js') },
];

// The API answers plain JSON, not {"data": …}.
export const mount = createMount({ routes, envelope: false });
