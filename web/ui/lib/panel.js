// usePanel returns the page context ({ api, client, params, query, navigate,
// href, setQuery, link }) built from the mount context, so nested
// components need not receive it through props.
import { createContext } from '../node_modules/@lsongdev/scripts/react/react.js';
import { useContext } from './html.js';

export const PanelContext = createContext(null);

export function usePanel() {
  const ctx = useContext(PanelContext);
  if (!ctx) throw new Error('usePanel must be used inside a page');
  return ctx;
}
