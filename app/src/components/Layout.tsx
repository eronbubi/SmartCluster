import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, Outlet } from 'react-router';
import { useCluster } from '../api/queries';
import { languages, setLanguage, type Language } from '../i18n';
import { applyTheme, storedTheme, type ThemePreference } from '../lib/theme';
import { useLiveStatus } from '../live/LiveContext';

export function Layout() {
  const { t, i18n } = useTranslation();
  const cluster = useCluster();
  const liveStatus = useLiveStatus();
  const [theme, setTheme] = useState<ThemePreference>(storedTheme);

  return (
    <div className="app">
      <header className="topbar">
        <Link to="/" className="brand">
          {t('app.title')}
          {cluster.data !== undefined && <span className="brand-cluster">{cluster.data.name}</span>}
        </Link>
        <div className="topbar-right">
          <span className={`live live-${liveStatus}`} role="status">
            <span className="badge-dot" aria-hidden="true" />
            {t(`live.${liveStatus}`)}
          </span>
          <label className="select">
            <span className="visually-hidden">{t('settings.language')}</span>
            <select
              value={i18n.language}
              onChange={(e) => {
                void setLanguage(e.target.value as Language);
              }}
            >
              {languages.map((l) => (
                <option key={l} value={l}>
                  {l.toUpperCase()}
                </option>
              ))}
            </select>
          </label>
          <label className="select">
            <span className="visually-hidden">{t('settings.theme')}</span>
            <select
              value={theme}
              onChange={(e) => {
                const pref = e.target.value as ThemePreference;
                applyTheme(pref);
                setTheme(pref);
              }}
            >
              <option value="system">{t('settings.themeSystem')}</option>
              <option value="light">{t('settings.themeLight')}</option>
              <option value="dark">{t('settings.themeDark')}</option>
            </select>
          </label>
        </div>
      </header>
      <main className="content">
        <Outlet />
      </main>
    </div>
  );
}
