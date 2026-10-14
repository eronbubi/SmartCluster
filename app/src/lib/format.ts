// Display formatting. Missing values (null/undefined) render as an en dash,
// never as 0, because a sensor that cannot be read is not a reading of 0.

export const missing = '–';

const byteUnits = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];

/** Bytes with 1024 steps and the unit names Windows uses. */
export function formatBytes(bytes: number | null | undefined, locale: string): string {
  if (bytes == null) return missing;
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < byteUnits.length - 1) {
    value /= 1024;
    unit += 1;
  }
  const digits = unit > 0 && value < 10 ? 1 : 0;
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: digits }).format(value)} ${byteUnits[unit] ?? 'B'}`;
}

/** Bytes per second as a byte rate (disks). */
export function formatByteRate(bytesPerSecond: number | null | undefined, locale: string): string {
  if (bytesPerSecond == null) return missing;
  return `${formatBytes(bytesPerSecond, locale)}/s`;
}

const bitUnits = ['bit/s', 'kbit/s', 'Mbit/s', 'Gbit/s'];

/** Bytes per second as a bit rate (network), like internet speeds are quoted. */
export function formatBitRate(bytesPerSecond: number | null | undefined, locale: string): string {
  if (bytesPerSecond == null) return missing;
  let value = bytesPerSecond * 8;
  let unit = 0;
  while (value >= 1000 && unit < bitUnits.length - 1) {
    value /= 1000;
    unit += 1;
  }
  const digits = unit > 0 && value < 10 ? 1 : 0;
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: digits }).format(value)} ${bitUnits[unit] ?? 'bit/s'}`;
}

/** A percentage given as 0..100. */
export function formatPercent(value: number | null | undefined, locale: string): string {
  if (value == null) return missing;
  return new Intl.NumberFormat(locale, { style: 'percent', maximumFractionDigits: 0 }).format(value / 100);
}

export function formatTemperature(celsius: number | null | undefined, locale: string): string {
  if (celsius == null) return missing;
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: 0 }).format(celsius)} °C`;
}

/** Share of used in total as 0..100, or null if total is unknown. */
export function usedPercent(used: number | null | undefined, total: number | null | undefined): number | null {
  if (used == null || total == null || total <= 0) return null;
  return Math.min(100, Math.max(0, (used / total) * 100));
}

export interface DurationLabels {
  days: string;
  hours: string;
  minutes: string;
}

/** Uptime as the two largest units, e.g. "3 d 4 h" or "12 min". */
export function formatUptime(seconds: number | null | undefined, labels: DurationLabels): string {
  if (seconds == null) return missing;
  const d = Math.floor(seconds / 86_400);
  const h = Math.floor((seconds % 86_400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d} ${labels.days} ${h} ${labels.hours}`;
  if (h > 0) return `${h} ${labels.hours} ${m} ${labels.minutes}`;
  return `${m} ${labels.minutes}`;
}

/** "5 minutes ago" in the given locale. */
export function formatRelative(iso: string | null | undefined, now: Date, locale: string): string {
  if (iso == null) return missing;
  const seconds = Math.round((new Date(iso).getTime() - now.getTime()) / 1000);
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
  const abs = Math.abs(seconds);
  if (abs < 60) return rtf.format(seconds, 'second');
  if (abs < 3600) return rtf.format(Math.round(seconds / 60), 'minute');
  if (abs < 86_400) return rtf.format(Math.round(seconds / 3600), 'hour');
  return rtf.format(Math.round(seconds / 86_400), 'day');
}

export function formatDateTime(iso: string | null | undefined, locale: string): string {
  if (iso == null) return missing;
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(iso));
}
