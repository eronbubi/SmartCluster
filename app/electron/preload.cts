// Runs sandboxed before the renderer; exposes only the control center address.
// Sandboxed preloads must be CommonJS, hence require.
// eslint-disable-next-line @typescript-eslint/no-require-imports -- CommonJS is required here
import electron = require('electron');

const { contextBridge, ipcRenderer } = electron;
const config = ipcRenderer.sendSync('smartcluster:config') as { controlUrl: string } | null;
if (config !== null) {
  contextBridge.exposeInMainWorld('smartcluster', Object.freeze({ controlUrl: config.controlUrl }));
}
