export {};

declare global {
  interface Window {
    /** Provided by the Electron preload script; absent in the web app. */
    readonly smartcluster?: { readonly controlUrl: string };
  }
}
