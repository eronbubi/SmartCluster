export type ThemePreference = 'system' | 'light' | 'dark';

const storageKey = 'smartcluster.theme';

export function storedTheme(): ThemePreference {
  try {
    const v = localStorage.getItem(storageKey);
    return v === 'light' || v === 'dark' ? v : 'system';
  } catch {
    return 'system';
  }
}

/** Applies the preference; "system" follows prefers-color-scheme via CSS. */
export function applyTheme(pref: ThemePreference): void {
  const root = document.documentElement;
  if (pref === 'system') delete root.dataset.theme;
  else root.dataset.theme = pref;
  try {
    localStorage.setItem(storageKey, pref);
  } catch {
    // not persisted, still applied
  }
}
