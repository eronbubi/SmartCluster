import { useTranslation } from 'react-i18next';

export function QueryError({ error, onRetry }: { error: Error; onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="card error" role="alert">
      <p>{t('common.loadError', { message: error.message })}</p>
      <button type="button" onClick={onRetry}>
        {t('common.retry')}
      </button>
    </div>
  );
}
