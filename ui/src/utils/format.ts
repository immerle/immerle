/** Human-friendly formatters shared across the UI. */
import { t } from '../i18n';

/** Seconds → `m:ss` or `h:mm:ss`. */
export function formatDuration(totalSeconds: number | undefined): string {
  if (!totalSeconds || totalSeconds < 0 || !Number.isFinite(totalSeconds)) return '0:00';
  const s = Math.floor(totalSeconds % 60);
  const m = Math.floor((totalSeconds / 60) % 60);
  const h = Math.floor(totalSeconds / 3600);
  const ss = s.toString().padStart(2, '0');
  if (h > 0) return `${h}:${m.toString().padStart(2, '0')}:${ss}`;
  return `${m}:${ss}`;
}

/** Bytes → a compact size using the active locale's units. */
export function formatBytes(bytes: number | undefined): string {
  const units = ['b', 'kb', 'mb', 'gb', 'tb'];
  if (!bytes || bytes <= 0 || !Number.isFinite(bytes)) return `0 ${t('format.bytes.b')}`;
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / 1024 ** i;
  return `${value.toFixed(i === 0 ? 0 : 1)} ${t(`format.bytes.${units[i]}`)}`;
}

/** Large integers → `12.3k`, `1.2M`. */
export function formatCount(n: number | undefined): string {
  if (!n) return '0';
  if (n < 1000) return String(n);
  if (n < 1_000_000) return `${(n / 1000).toFixed(1)}k`;
  return `${(n / 1_000_000).toFixed(1)}M`;
}

/** ISO date → compact relative time in the active locale. */
export function formatRelativeTime(iso: string | undefined): string {
  if (!iso) return '';
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then)) return '';
  const diff = Math.max(0, Date.now() - then);
  const min = Math.floor(diff / 60_000);
  if (min < 1) return t('format.relative.now');
  if (min < 60) return t('format.relative.minute', { count: min });
  const h = Math.floor(min / 60);
  if (h < 24) return t('format.relative.hour', { count: h });
  const d = Math.floor(h / 24);
  if (d < 7) return t('format.relative.day', { count: d });
  const w = Math.floor(d / 7);
  if (w < 5) return t('format.relative.week', { count: w });
  const mo = Math.floor(d / 30);
  if (mo < 12) return t('format.relative.month', { count: mo });
  return t('format.relative.year', { count: Math.floor(d / 365) });
}
