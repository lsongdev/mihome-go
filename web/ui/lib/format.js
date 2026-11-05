export function bytes(n) {
  if (n == null || isNaN(n)) return '—';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; }
  return `${n < 10 && i > 0 ? n.toFixed(1) : Math.round(n)} ${units[i]}`;
}

export function percent(n) {
  return n == null ? '—' : `${n.toFixed(n < 10 ? 1 : 0)}%`;
}

const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' });

// ago formats a date (Date, ISO string or unix seconds) relative to now.
export function ago(value) {
  const date = toDate(value);
  if (!date) return '—';
  const seconds = (date.getTime() - Date.now()) / 1000;
  const steps = [[60, 'second'], [3600, 'minute'], [86400, 'hour'], [2592000, 'day'], [31536000, 'month'], [Infinity, 'year']];
  const divisors = { second: 1, minute: 60, hour: 3600, day: 86400, month: 2592000, year: 31536000 };
  for (const [limit, unit] of steps) {
    if (Math.abs(seconds) < limit) return rtf.format(Math.round(seconds / divisors[unit]), unit);
  }
  return '—';
}

const pad = n => String(n).padStart(2, '0');

// datetime formats a date as local "2006-01-02 15:04:05".
export function datetime(value) {
  const d = toDate(value);
  return d ? `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}` : '—';
}

function toDate(value) {
  if (value == null || value === '') return null;
  const date = typeof value === 'number' ? new Date(value * 1000) : new Date(value);
  return isNaN(date) || date.getFullYear() < 1971 ? null : date;
}

export function shortId(id = '') {
  return id.replace(/^sha256:/, '').slice(0, 12);
}

// daysUntil returns whole days from now until value (negative if past).
export function daysUntil(value) {
  const date = value ? new Date(value) : null;
  return date && !isNaN(date) ? Math.floor((date.getTime() - Date.now()) / 86400000) : null;
}
