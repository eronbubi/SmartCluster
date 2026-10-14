import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import type { Node, Stats } from '../api/types';
import { formatBitRate, formatBytes, formatPercent, formatRelative, formatTemperature, usedPercent } from '../lib/format';
import { useLiveNode } from '../live/LiveContext';
import { Meter } from './Meter';
import { Sparkline } from './Sparkline';
import { StatusBadge } from './StatusBadge';

/** The fullest mounted disk, which is the one that runs out first. */
export function fullestDisk(stats: Stats) {
  let best: { mount: string; percent: number } | null = null;
  for (const d of stats.disks) {
    const p = usedPercent(d.used, d.total);
    if (p !== null && (best === null || p > best.percent)) best = { mount: d.mount, percent: p };
  }
  return best;
}

export function NodeCard({ node, now }: { node: Node; now: Date }) {
  const { t, i18n } = useTranslation();
  const lng = i18n.language;
  const live = useLiveNode(node.id);
  const latest = live?.latest ?? node.latest;
  const offline = node.status !== 'online';

  return (
    <article className={`card node-card${offline ? ' is-offline' : ''}`}>
      <header className="node-card-head">
        <div>
          <h3>
            <Link to={`/nodes/${node.id}`}>{node.name}</Link>
          </h3>
          <p className="muted">{node.os?.name ?? node.hostname}</p>
        </div>
        <StatusBadge status={node.status} />
      </header>

      {offline && (
        <p className="muted">
          {node.last_seen_at === null
            ? t('node.neverConnected')
            : t('node.lastSeen', { when: formatRelative(node.last_seen_at, now, lng) })}
        </p>
      )}

      {latest === null ? (
        !offline && <p className="muted">{t('node.noStats')}</p>
      ) : (
        <NodeMetrics stats={latest} cpuHistory={live?.cpu ?? []} />
      )}
    </article>
  );
}

function NodeMetrics({ stats, cpuHistory }: { stats: Stats; cpuHistory: readonly number[] }) {
  const { t, i18n } = useTranslation();
  const lng = i18n.language;
  const disk = fullestDisk(stats);
  const rx = stats.net.reduce((sum, n) => sum + n.rx_bps, 0);
  const tx = stats.net.reduce((sum, n) => sum + n.tx_bps, 0);
  const memPct = usedPercent(stats.mem.used, stats.mem.total);
  const gpu = stats.gpus.find((g) => g.util_pct !== null);

  return (
    <div className="node-metrics">
      <div className="cpu-row">
        <Meter label={t('metric.cpu')} percent={stats.cpu.pct} text={formatPercent(stats.cpu.pct, lng)} />
        <Sparkline values={cpuHistory} />
      </div>
      <Meter
        label={t('metric.memory')}
        percent={memPct}
        text={`${formatBytes(stats.mem.used, lng)} / ${formatBytes(stats.mem.total, lng)}`}
      />
      {disk !== null && (
        <Meter label={`${t('metric.disk')} ${disk.mount}`} percent={disk.percent} text={formatPercent(disk.percent, lng)} />
      )}
      <dl className="facts">
        <div>
          <dt>{t('metric.network')}</dt>
          <dd>
            ↓ {formatBitRate(rx, lng)} · ↑ {formatBitRate(tx, lng)}
          </dd>
        </div>
        {stats.cpu.temp_c !== null && (
          <div>
            <dt>{t('metric.temperature')}</dt>
            <dd>{formatTemperature(stats.cpu.temp_c, lng)}</dd>
          </div>
        )}
        {gpu !== undefined && (
          <div>
            <dt>{t('metric.gpu')}</dt>
            <dd>{formatPercent(gpu.util_pct, lng)}</dd>
          </div>
        )}
        {stats.battery !== null && !stats.battery.on_ac && (
          <div className="on-battery">
            <dt>{t('metric.battery')}</dt>
            <dd>
              {t('node.onBattery')} ({formatPercent(stats.battery.pct, lng)})
            </dd>
          </div>
        )}
      </dl>
    </div>
  );
}
