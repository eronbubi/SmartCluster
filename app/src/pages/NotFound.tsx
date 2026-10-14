import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

export function NotFound() {
  const { t } = useTranslation();
  return (
    <section className="card">
      <p>404</p>
      <Link to="/">{t('node.back')}</Link>
    </section>
  );
}
