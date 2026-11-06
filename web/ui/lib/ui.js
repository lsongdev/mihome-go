// The @lsongdev/scripts components the pages use, from one place, plus
// toastError for failed actions.
import { toast } from '../node_modules/@lsongdev/scripts/react/components/index.js';

export {
  Alert, Badge, Loading, Empty, KeyValue, PageHeader, Navbar, Field, Check, ListInput, Tabs, LogView,
  CopyButton, ConfirmButton, Toaster, toast, openDialog, DialogHost, Table, ProgressBar,
} from '../node_modules/@lsongdev/scripts/react/components/index.js';

export function toastError(err) {
  toast(err?.message || String(err), 'danger');
}
