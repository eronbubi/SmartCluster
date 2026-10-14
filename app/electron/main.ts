// Electron shell of the control app (ADR-0020). The renderer is bundled and
// served through the app:// protocol (no network server); it talks only to
// the configured control center, whose certificate must come from the pinned
// cluster CA.
import { app, BrowserWindow, dialog, ipcMain, net, protocol, session } from 'electron';
import { readFileSync, statSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { verifyServerCertificate } from './certverify.js';
import { loadConfig, type ShellConfig } from './config.js';
import { contentSecurityPolicy, resolveAppFile } from './paths.js';

const appOrigin = 'app://smartcluster';
const here = path.dirname(fileURLToPath(import.meta.url));
const distDir = path.resolve(here, '..', 'dist');

protocol.registerSchemesAsPrivileged([
  { scheme: 'app', privileges: { standard: true, secure: true, supportFetchAPI: true } },
]);

function isFile(p: string): boolean {
  try {
    return statSync(p).isFile();
  } catch {
    return false;
  }
}

function serveRenderer(config: ShellConfig): void {
  const csp = contentSecurityPolicy(config.controlUrl);
  protocol.handle('app', async (request) => {
    const file = resolveAppFile(distDir, request.url, isFile);
    const res = await net.fetch(pathToFileURL(file).toString());
    const headers = new Headers(res.headers);
    headers.set('Content-Security-Policy', csp);
    return new Response(res.body, { status: res.status, headers });
  });
}

function pinControlCenter(config: ShellConfig): void {
  session.defaultSession.setCertificateVerifyProc((req, callback) => {
    const ok =
      req.hostname === config.controlHost &&
      verifyServerCertificate(req.certificate.data, config.caPem, req.hostname, new Date());
    callback(ok ? 0 : -2); // 0 = accept, -2 = reject
  });
  // No permissions (camera, notifications, ...) are needed in milestone a.
  session.defaultSession.setPermissionRequestHandler((_wc, _permission, callback) => {
    callback(false);
  });
}

function lockDown(): void {
  app.on('web-contents-created', (_event, contents) => {
    contents.on('will-navigate', (event, url) => {
      if (!url.startsWith(`${appOrigin}/`)) event.preventDefault();
    });
    contents.setWindowOpenHandler(() => ({ action: 'deny' }));
  });
}

function createWindow(): void {
  const win = new BrowserWindow({
    width: 1280,
    height: 820,
    minWidth: 720,
    minHeight: 500,
    title: 'SmartCluster',
    autoHideMenuBar: true,
    webPreferences: {
      preload: path.join(here, 'preload.cjs'),
      contextIsolation: true,
      sandbox: true,
      nodeIntegration: false,
    },
  });
  void win.loadURL(`${appOrigin}/`);
}

function start(): void {
  const result = loadConfig(process.argv, process.env, (p) => readFileSync(p, 'utf8'));
  if (!result.ok) {
    dialog.showErrorBox('SmartCluster', result.error);
    app.quit();
    return;
  }
  const config = result.config;
  ipcMain.on('smartcluster:config', (event) => {
    // Only our own renderer may ask.
    event.returnValue = event.senderFrame?.url.startsWith(`${appOrigin}/`) ? { controlUrl: config.controlUrl } : null;
  });
  lockDown();
  serveRenderer(config);
  pinControlCenter(config);
  createWindow();
}

void app.whenReady().then(start);

app.on('window-all-closed', () => {
  app.quit();
});
