import { describe, expect, it } from 'vitest';
import {
  formatBitRate,
  formatByteRate,
  formatBytes,
  formatPercent,
  formatRelative,
  formatTemperature,
  formatUptime,
  missing,
  usedPercent,
} from './format';

const labels = { days: 'd', hours: 'h', minutes: 'min' };

describe('format', () => {
  it('formats bytes with 1024 steps', () => {
    expect(formatBytes(0, 'en')).toBe('0 B');
    expect(formatBytes(1536, 'en')).toBe('1.5 KB');
    expect(formatBytes(16_962_445_312, 'en')).toBe('16 GB');
    expect(formatBytes(16_962_445_312, 'de')).toBe('16 GB');
    expect(formatBytes(5 * 1024 ** 3 + 300 * 1024 ** 2, 'de')).toBe('5,3 GB');
    expect(formatByteRate(2048, 'en')).toBe('2 KB/s');
  });

  it('formats network rates in bits', () => {
    expect(formatBitRate(125_000, 'en')).toBe('1 Mbit/s');
    expect(formatBitRate(5453, 'en')).toBe('44 kbit/s');
    expect(formatBitRate(1000, 'en')).toBe('8 kbit/s');
    expect(formatBitRate(0, 'en')).toBe('0 bit/s');
  });

  it('formats percentages and temperatures per locale', () => {
    expect(formatPercent(12.4, 'en')).toBe('12%');
    expect(formatPercent(12.4, 'de')).toMatch(/^12\s%$/);
    expect(formatTemperature(52.4, 'de')).toBe('52 °C');
  });

  it('shows missing values as a dash, never as 0', () => {
    for (const f of [formatBytes, formatBitRate, formatPercent, formatTemperature]) {
      expect(f(null, 'de')).toBe(missing);
      expect(f(undefined, 'en')).toBe(missing);
    }
    expect(formatUptime(null, labels)).toBe(missing);
    expect(usedPercent(5, null)).toBeNull();
    expect(usedPercent(5, 0)).toBeNull();
  });

  it('computes used percentages within 0..100', () => {
    expect(usedPercent(50, 200)).toBe(25);
    expect(usedPercent(300, 200)).toBe(100);
  });

  it('formats uptime with the two largest units', () => {
    expect(formatUptime(30_059, labels)).toBe('8 h 20 min');
    expect(formatUptime(3 * 86_400 + 4 * 3600 + 5, labels)).toBe('3 d 4 h');
    expect(formatUptime(59, labels)).toBe('0 min');
  });

  it('formats relative times', () => {
    const now = new Date('2026-10-14T12:00:00Z');
    expect(formatRelative('2026-10-14T11:55:00Z', now, 'en')).toBe('5 minutes ago');
    expect(formatRelative('2026-10-14T09:00:00Z', now, 'de')).toBe('vor 3 Stunden');
  });
});
