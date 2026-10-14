// @vitest-environment node
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import { verifyServerCertificate } from './certverify.js';
import { loadConfig } from './config.js';
import { contentSecurityPolicy, resolveAppFile } from './paths.js';

const fixture = (name: string) => readFileSync(fileURLToPath(new URL(`./testdata/${name}`, import.meta.url)), 'utf8');
const ca = fixture('ca.pem');
const now = new Date('2026-10-14T12:00:00Z');

describe('verifyServerCertificate', () => {
  it('accepts the control center certificate for its addresses', () => {
    expect(verifyServerCertificate(fixture('server.pem'), ca, '192.168.1.20', now)).toBe(true);
    expect(verifyServerCertificate(fixture('server.pem'), ca, 'control.local', now)).toBe(true);
  });

  it('rejects other hosts, other CAs, expired and client certificates', () => {
    expect(verifyServerCertificate(fixture('server.pem'), ca, '192.168.1.21', now)).toBe(false);
    expect(verifyServerCertificate(fixture('server.pem'), ca, 'evil.example', now)).toBe(false);
    expect(verifyServerCertificate(fixture('foreign-server.pem'), ca, '192.168.1.20', now)).toBe(false);
    expect(verifyServerCertificate(fixture('expired.pem'), ca, '192.168.1.20', now)).toBe(false);
    expect(verifyServerCertificate(fixture('client.pem'), ca, '192.168.1.20', now)).toBe(false);
    expect(verifyServerCertificate('garbage', ca, '192.168.1.20', now)).toBe(false);
    // A leaf certificate pinned in place of the CA is not a CA.
    expect(verifyServerCertificate(fixture('server.pem'), fixture('server.pem'), '192.168.1.20', now)).toBe(false);
  });
});

describe('loadConfig', () => {
  const read = (p: string) => {
    if (p === 'ca.crt') return ca;
    if (p === 'notpem.txt') return 'hello';
    throw new Error('ENOENT');
  };

  it('reads arguments and falls back to the environment', () => {
    const fromArgs = loadConfig(['electron', '.', '--control=https://192.168.1.20:8443', '--ca=ca.crt'], {}, read);
    expect(fromArgs).toEqual({ ok: true, config: { controlUrl: 'https://192.168.1.20:8443', controlHost: '192.168.1.20', caPem: ca } });
    const fromEnv = loadConfig([], { SC_CONTROL_URL: 'https://[::1]:8443', SC_CA_FILE: 'ca.crt' }, read);
    expect(fromEnv.ok && fromEnv.config.controlHost).toBe('::1');
  });

  it('explains what is wrong', () => {
    const err = (argv: string[]) => {
      const r = loadConfig(argv, {}, read);
      return r.ok ? '' : r.error;
    };
    expect(err([])).toContain('--control=');
    expect(err(['--control=http://192.168.1.20:8443', '--ca=ca.crt'])).toContain('https://');
    expect(err(['--control=https://192.168.1.20:8443/api', '--ca=ca.crt'])).toContain('only scheme, host and port');
    expect(err(['--control=https://192.168.1.20:8443', '--ca=missing.crt'])).toContain('Cannot read');
    expect(err(['--control=https://192.168.1.20:8443', '--ca=notpem.txt'])).toContain('not a PEM');
  });
});

describe('resolveAppFile', () => {
  const dist = path.resolve('/tmp/dist');
  const files = new Set([path.join(dist, 'index.html'), path.join(dist, 'assets', 'app-1.js')]);
  const isFile = (p: string) => files.has(p);

  it('serves files and falls back to index.html for routes', () => {
    expect(resolveAppFile(dist, 'app://smartcluster/assets/app-1.js', isFile)).toBe(path.join(dist, 'assets', 'app-1.js'));
    expect(resolveAppFile(dist, 'app://smartcluster/nodes/abc', isFile)).toBe(path.join(dist, 'index.html'));
    expect(resolveAppFile(dist, 'app://smartcluster/', isFile)).toBe(path.join(dist, 'index.html'));
  });

  it('never leaves the dist directory', () => {
    for (const url of ['app://smartcluster/../../etc/passwd', 'app://smartcluster/%2e%2e/%2e%2e/secret', 'app://smartcluster/%E0%A4%A']) {
      expect(resolveAppFile(dist, url, () => true).startsWith(dist + path.sep)).toBe(true);
    }
  });
});

describe('contentSecurityPolicy', () => {
  it('allows only the control center on the network', () => {
    const csp = contentSecurityPolicy('https://192.168.1.20:8443');
    expect(csp).toContain('connect-src https://192.168.1.20:8443 wss://192.168.1.20:8443');
    expect(csp).toContain("script-src 'self'");
  });
});
