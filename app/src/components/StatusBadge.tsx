import { useTranslation } from 'react-i18next';
import type { NodeStatus } from '../api/types';

export function StatusBadge({ status }: { status: NodeStatus }) {
  const { t } = useTranslation();
  return (
    <span className={`badge badge-${status}`} data-testid="status">
      <span className="badge-dot" aria-hidden="true" />
      {t(`status.${status}`)}
    </span>
  );
}
