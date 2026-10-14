import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import { de, type Resources } from './de';
import { en } from './en';

export const languages = ['de', 'en'] as const;
export type Language = (typeof languages)[number];

const storageKey = 'smartcluster.language';

declare module 'i18next' {
  interface CustomTypeOptions {
    defaultNS: 'translation';
    resources: { translation: Resources };
  }
}

function storedLanguage(): Language | null {
  try {
    const v = localStorage.getItem(storageKey);
    return v === 'de' || v === 'en' ? v : null;
  } catch {
    return null; // storage blocked
  }
}

export function initialLanguage(): Language {
  return storedLanguage() ?? (navigator.language.toLowerCase().startsWith('de') ? 'de' : 'en');
}

export async function setLanguage(lng: Language): Promise<void> {
  try {
    localStorage.setItem(storageKey, lng);
  } catch {
    // not persisted, still switch
  }
  document.documentElement.lang = lng;
  await i18n.changeLanguage(lng);
}

void i18n.use(initReactI18next).init({
  resources: { de: { translation: de }, en: { translation: en } },
  lng: initialLanguage(),
  fallbackLng: 'en',
  interpolation: { escapeValue: false }, // React escapes
});

export default i18n;
