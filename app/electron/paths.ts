import path from 'node:path';

/**
 * Maps an app://smartcluster/... URL to a file in the built renderer.
 * Unknown paths (client-side routes like /nodes/<id>) and anything that would
 * leave distDir get index.html.
 */
export function resolveAppFile(distDir: string, requestUrl: string, isFile: (p: string) => boolean): string {
  const index = path.join(distDir, 'index.html');
  let rel: string;
  try {
    rel = decodeURIComponent(new URL(requestUrl).pathname);
  } catch {
    return index;
  }
  const file = path.resolve(distDir, `.${path.posix.normalize(`/${rel}`)}`);
  if (!file.startsWith(distDir + path.sep) || !isFile(file)) return index;
  return file;
}

/** Content-Security-Policy for the renderer: own files, and only the control center on the network. */
export function contentSecurityPolicy(controlUrl: string): string {
  const ws = controlUrl.replace(/^https/, 'wss');
  return [
    "default-src 'self'",
    "script-src 'self'",
    "style-src 'self'",
    "img-src 'self' data:",
    `connect-src ${controlUrl} ${ws}`,
    "frame-ancestors 'none'",
    "base-uri 'self'",
    "form-action 'none'",
  ].join('; ');
}
