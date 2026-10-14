export interface ShellConfig {
  /** Origin of the control center API, e.g. https://192.168.1.20:8443 */
  controlUrl: string;
  /** Host name or IP without brackets, as certificate checks compare it. */
  controlHost: string;
  caPem: string;
}

export type ConfigResult = { ok: true; config: ShellConfig } | { ok: false; error: string };

const usage =
  'Start the app with --control=https://<address>:8443 --ca=<path to ca.crt> ' +
  '(or SC_CONTROL_URL and SC_CA_FILE). The setup wizard replaces this in milestone b.';

/** Reads the shell configuration from the command line or the environment. */
export function loadConfig(
  argv: readonly string[],
  env: Readonly<Record<string, string | undefined>>,
  readFile: (path: string) => string,
): ConfigResult {
  const arg = (name: string) => argv.find((a) => a.startsWith(`--${name}=`))?.slice(name.length + 3);
  const control = arg('control') ?? env.SC_CONTROL_URL;
  const caFile = arg('ca') ?? env.SC_CA_FILE;
  if (control === undefined || control === '' || caFile === undefined || caFile === '') {
    return { ok: false, error: usage };
  }

  let url: URL;
  try {
    url = new URL(control);
  } catch {
    return { ok: false, error: `Not a valid address: ${control}` };
  }
  if (url.protocol !== 'https:') return { ok: false, error: 'The control center address must start with https://' };
  if (url.pathname !== '/' || url.search !== '' || url.hash !== '' || url.username !== '' || url.password !== '') {
    return { ok: false, error: 'Give only scheme, host and port, e.g. https://192.168.1.20:8443' };
  }

  let caPem: string;
  try {
    caPem = readFile(caFile);
  } catch {
    return { ok: false, error: `Cannot read the CA certificate ${caFile}` };
  }
  if (!caPem.includes('-----BEGIN CERTIFICATE-----')) {
    return { ok: false, error: `${caFile} is not a PEM certificate` };
  }
  return { ok: true, config: { controlUrl: url.origin, controlHost: url.hostname.replace(/^\[|\]$/g, ''), caPem } };
}
