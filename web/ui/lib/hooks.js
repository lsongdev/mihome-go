import { useCallback, useEffect, useRef, useState } from './html.js';

// useResource loads data with fn(signal) and reloads when deps change.
// Reloads keep showing previous data instead of flashing a spinner.
// interval (ms, or a function of the latest data returning ms) polls while
// it is positive.
export function useResource(fn, deps = [], { interval } = {}) {
  const [state, setState] = useState({ data: undefined, error: null, loading: true });
  const [version, setVersion] = useState(0);
  const intervalRef = useRef(interval);
  intervalRef.current = interval;
  useEffect(() => {
    const controller = new AbortController();
    let timer;
    const load = async () => {
      let data;
      try {
        data = await fn(controller.signal);
        setState({ data, error: null, loading: false });
      } catch (error) {
        if (error.name === 'AbortError') return;
        setState(s => ({ ...s, error, loading: false }));
      }
      const i = intervalRef.current;
      const delay = typeof i === 'function' ? i(data) : i;
      if (delay > 0 && !controller.signal.aborted) timer = setTimeout(load, delay);
    };
    setState(s => ({ ...s, loading: true }));
    load();
    return () => { controller.abort(); clearTimeout(timer); };
  }, [...deps, version]);
  const reload = useCallback(() => setVersion(v => v + 1), []);
  return { ...state, reload };
}

// useBusy tracks in-flight actions by key and reports their errors.
export function useBusy(onError) {
  const [busy, setBusy] = useState({});
  const run = useCallback(async (key, fn) => {
    setBusy(b => ({ ...b, [key]: true }));
    try {
      return await fn();
    } catch (err) {
      onError?.(err);
    } finally {
      setBusy(b => { const next = { ...b }; delete next[key]; return next; });
    }
  }, [onError]);
  return [busy, run];
}

// useAction wraps one async operation with busy and error state, for forms
// and buttons whose failure is shown inline.
export function useAction() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const pending = useRef(false);
  const run = useCallback(async fn => {
    if (pending.current) return undefined;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      return await fn();
    } catch (err) {
      setError(err);
      return undefined;
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }, []);
  return { busy, error, run, setError };
}
