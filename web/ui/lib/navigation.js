// Hash navigation shared by the panel and standalone hosts. Only accepted
// routes are published; cancelling Back restores the original history entry.
export function createNavigation(win = window) {
  const key = 'panelNavigation';
  const previous = win.history.state?.[key];
  const session = previous?.session || Math.random().toString(36).slice(2);
  let index = previous?.index || 0;
  let current = win.location.href;
  let restoring = false;
  const guards = new Set();
  const state = () => ({ ...win.history.state, [key]: { session, index } });
  win.history.replaceState(state(), '', current);

  const canLeave = () => [...guards].every(guard => guard());
  const publish = () => win.dispatchEvent(new Event('panel:routechange'));
  const navigate = (to, { replace = false } = {}) => {
    const url = new URL('#' + to, current).href;
    if (url === current) return true;
    if (restoring || !canLeave()) return false;
    if (!replace) index++;
    win.history[replace ? 'replaceState' : 'pushState'](state(), '', url);
    current = url;
    publish();
    return true;
  };

  const changed = () => {
    if (win.location.href === current) { restoring = false; return; }
    if (restoring) return;
    const entry = win.history.state?.[key];
    const next = entry?.session === session ? entry.index : index + 1;
    if (!canLeave()) {
      restoring = true;
      win.history.go(index - next);
      return;
    }
    index = next;
    current = win.location.href;
    win.history.replaceState(state(), '', current);
    publish();
  };
  const click = event => {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    // composedPath also finds anchors inside a module's shadow root.
    const link = event.composedPath().find(el => el.tagName === 'A' && el.href);
    if (!link || link.hasAttribute('download') || (link.target && link.target !== '_self')) return;
    const url = new URL(link.href, current);
    const here = new URL(current);
    if (url.origin !== here.origin || url.pathname !== here.pathname || url.search !== here.search || !url.hash.startsWith('#/')) return;
    event.preventDefault();
    navigate(url.hash.slice(1));
  };
  win.addEventListener('click', click);
  win.addEventListener('hashchange', changed);
  win.addEventListener('popstate', changed);
  return {
    navigate,
    canLeave,
    beforeLeave(guard) { guards.add(guard); return () => guards.delete(guard); },
    dispose() {
      win.removeEventListener('click', click);
      win.removeEventListener('hashchange', changed);
      win.removeEventListener('popstate', changed);
      guards.clear();
    },
  };
}
