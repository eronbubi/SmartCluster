import { useTranslation } from 'react-i18next';
import { useNodes } from '../api/queries';
import type { Node } from '../api/types';
import { NodeCard } from '../components/NodeCard';
import { QueryError } from '../components/QueryError';
import { formatBytes, formatPercent } from '../lib/format';
import { useNow } from '../lib/useNow';
import { useLiveSnapshot } from '../live/LiveContext';

export function Dashboard() {
  const { t } = useTranslation();
  const nodes = useNodes();
  const now = useNow();

  if (nodes.isPending) return <p className="muted">{t('common.loading')}</p>;
  if (nodes.isError) {
    return (
      <QueryError
        error={nodes.error}
        onRetry={() => {
          void nodes.refetch();
        }}
      />
    );
  }
  if (nodes.data.length === 0) return <EmptyState />;

  return (
    <>
      <Summary nodes={nodes.data} />
      <h2>{t('dashboard.title')}</h2>
      <div className="grid">
        {nodes.data.map((n) => (
          <NodeCard key={n.id} node={n} now={now} />
        ))}
      </div>
    </>
  );
}

function Summary({ nodes }: { nodes: Node[] }) {
  const { t, i18n } = useTranslation();
  const lng = i18n.language;
  const live = useLiveSnapshot();
  const online = nodes.filter((n) => n.status === 'online');
  const latest = online.flatMap((n) => {
    const s = live.get(n.id)?.latest ?? n.latest;
    return s === null ? [] : [s];
  });
  const cpu = latest.length === 0 ? null : latest.reduce((sum, s) => sum + s.cpu.pct, 0) / latest.length;
  const memUsed = latest.reduce((sum, s) => sum + s.mem.used, 0);
  const memTotal = latest.reduce((sum, s) => sum + s.mem.total, 0);

  return (
    <section className="summary" aria-label={t('app.overview')}>
      <div className="summary-item">
        <span className="summary-value">{t('dashboard.online', { online: online.length, total: nodes.length })}</span>
      </div>
      <div className="summary-item">
        <span className="summary-label">{t('dashboard.cpuAverage')}</span>
        <span className="summary-value">{formatPercent(cpu, lng)}</span>
      </div>
      <div className="summary-item">
        <span className="summary-label">{t('dashboard.memory')}</span>
        <span className="summary-value">
          {latest.length === 0 ? formatBytes(null, lng) : `${formatBytes(memUsed, lng)} / ${formatBytes(memTotal, lng)}`}
        </span>
      </div>
    </section>
  );
}

function EmptyState() {
  const { t } = useTranslation();
  return (
    <section className="card empty">
      <h2>{t('dashboard.emptyTitle')}</h2>
      <p>{t('dashboard.emptyText')}</p>
      <ol>
        <li>
          {t('dashboard.emptyStep1')}
          <pre>
            <code>smartcluster-control token create</code>
          </pre>
        </li>
        <li>
          {t('dashboard.emptyStep2')}
          <pre>
            <code>smartcluster-agent join &lt;CODE&gt;</code>
            <br />
            <code>smartcluster-agent service install</code>
          </pre>
        </li>
      </ol>
    </section>
  );
}
