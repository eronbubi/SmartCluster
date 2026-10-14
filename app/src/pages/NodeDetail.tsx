import { lazy, Suspense, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useParams } from 'react-router';
import { ApiError } from '../api/client';
import { ranges, useNode, useNodeStats, type RangeKey } from '../api/queries';
import type { Node, Point, Stats } from '../api/types';
import { Meter } from '../components/Meter';
import { QueryError } from '../components/QueryError';
import type { ChartSeries } from '../components/StatsChart';
import { StatusBadge } from '../components/StatusBadge';
import {
  formatBitRate,
  formatByteRate,
  formatBytes,
  formatDateTime,
  formatPercent,
  formatRelative,
  formatTemperature,
  formatUptime,
  missing,
  usedPercent,
} from '../lib/format';
import { useNow } from '../lib/useNow';
import { useLiveNode } from '../live/LiveContext';

// uPlot is only needed here; loading it on demand keeps the overview light.
const StatsChart = lazy(async () => ({ default: (await import('../components/StatsChart')).StatsChart }));

export function NodeDetail() {
  const { t } = useTranslation();
  const { id = '' } = useParams();
  const node = useNode(id);

  if (node.isPending) return <p className="muted">{t('common.loading')}</p>;
  if (node.isError) {
    if (node.error instanceof ApiError && node.error.status === 404) {
      return (
        <section className="card">
          <p>{t('node.notFound')}</p>
          <Link to="/">{t('node.back')}</Link>
        </section>
      );
    }
    return (
      <QueryError
        error={node.error}
        onRetry={() => {
          void node.refetch();
        }}
      />
    );
  }
  return <NodeView node={node.data} />;
}

function NodeView({ node }: { node: Node }) {
  const { t, i18n } = useTranslation();
  const lng = i18n.language;
  const now = useNow();
  const live = useLiveNode(node.id);
  const latest = live?.latest ?? node.latest;

  return (
    <>
      <Link to="/" className="back">
        ← {t('node.back')}
      </Link>
      <header className="detail-head">
        <h2>{node.name}</h2>
        <StatusBadge status={node.status} />
        {node.status !== 'online' && node.last_seen_at !== null && (
          <span className="muted">{t('node.lastSeen', { when: formatRelative(node.last_seen_at, now, lng) })}</span>
        )}
      </header>

      <div className="detail-grid">
        <SystemCard node={node} />
        <HardwareCard node={node} />
        <NetworkCard node={node} />
        {latest !== null && <NowCard stats={latest} />}
      </div>

      <History nodeId={node.id} />
    </>
  );
}

function Field({ label, value }: { label: string; value: string | undefined }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{value === undefined || value === '' ? missing : value}</dd>
    </div>
  );
}

function SystemCard({ node }: { node: Node }) {
  const { t, i18n } = useTranslation();
  const os = node.os;
  return (
    <section className="card">
      <h3>{t('detail.system')}</h3>
      <dl className="fields">
        <Field label={t('detail.operatingSystem')} value={os === null ? undefined : `${os.name} ${os.version}`.trim()} />
        <Field label={t('detail.build')} value={os?.build} />
        <Field label={t('detail.kernel')} value={os?.kernel} />
        <Field label={t('detail.architecture')} value={os?.arch} />
        <Field label={t('detail.hostname')} value={node.hostname} />
        <Field label={t('detail.agentVersion')} value={node.agent_version} />
        <Field label={t('detail.certificate')} value={formatDateTime(node.cert_not_after, i18n.language)} />
      </dl>
    </section>
  );
}

function HardwareCard({ node }: { node: Node }) {
  const { t, i18n } = useTranslation();
  const hw = node.hardware;
  if (hw === null) return null;
  const model = [hw.model, hw.product_version].filter((s) => s !== undefined && s !== '').join(' · ');
  return (
    <section className="card">
      <h3>{t('detail.hardware')}</h3>
      <dl className="fields">
        <Field label={t('detail.manufacturer')} value={hw.vendor} />
        <Field label={t('detail.model')} value={model} />
        <Field label={t('detail.bios')} value={hw.bios_version} />
        <Field
          label={t('detail.processor')}
          value={`${hw.cpu.model} (${t('detail.cores', { cores: hw.cpu.cores, threads: hw.cpu.threads })})`}
        />
        <Field label={t('detail.memoryTotal')} value={formatBytes(hw.ram_total, i18n.language)} />
        <Field
          label={t('detail.graphics')}
          value={hw.gpus.length === 0 ? t('detail.noGraphics') : hw.gpus.map((g) => g.name).join(', ')}
        />
      </dl>
    </section>
  );
}

function NetworkCard({ node }: { node: Node }) {
  const { t } = useTranslation();
  const nics = node.hardware?.nics ?? [];
  if (nics.length === 0) return null;
  return (
    <section className="card">
      <h3>{t('detail.network')}</h3>
      <ul className="plain">
        {nics.map((n) => (
          <li key={n.name}>
            <strong>{n.name}</strong> <span className="muted mono">{n.mac}</span>
            <ul className="plain mono small">
              {n.addrs.map((a) => (
                <li key={a}>{a}</li>
              ))}
            </ul>
          </li>
        ))}
      </ul>
    </section>
  );
}

function NowCard({ stats }: { stats: Stats }) {
  const { t, i18n } = useTranslation();
  const lng = i18n.language;
  const units = { days: t('units.days'), hours: t('units.hours'), minutes: t('units.minutes') };
  return (
    <section className="card now">
      <h3>{t('detail.now')}</h3>
      <Meter label={t('metric.cpu')} percent={stats.cpu.pct} text={formatPercent(stats.cpu.pct, lng)} />
      {stats.cpu.per_core.length > 1 && (
        <div className="cores" aria-label={t('detail.perCore')}>
          {stats.cpu.per_core.map((p, i) => (
            <span key={i} className="core" title={formatPercent(p, lng)} style={{ height: `${Math.max(4, p)}%` }} />
          ))}
        </div>
      )}
      <Meter
        label={t('metric.memory')}
        percent={usedPercent(stats.mem.used, stats.mem.total)}
        text={`${formatBytes(stats.mem.used, lng)} / ${formatBytes(stats.mem.total, lng)}`}
      />
      {stats.mem.swap_total > 0 && (
        <Meter
          label={t('metric.swap')}
          percent={usedPercent(stats.mem.swap_used, stats.mem.swap_total)}
          text={`${formatBytes(stats.mem.swap_used, lng)} / ${formatBytes(stats.mem.swap_total, lng)}`}
        />
      )}
      {stats.disks.map((d) => (
        <div key={d.mount}>
          <Meter
            label={`${t('metric.disk')} ${d.mount}`}
            percent={usedPercent(d.used, d.total)}
            text={`${formatBytes(d.used, lng)} / ${formatBytes(d.total, lng)}`}
          />
          <p className="muted small">
            {t('metric.read')} {formatByteRate(d.read_bps, lng)} · {t('metric.write')} {formatByteRate(d.write_bps, lng)}
          </p>
        </div>
      ))}
      <dl className="fields">
        {stats.net.map((n) => (
          <Field
            key={n.iface}
            label={`${t('metric.network')} ${n.iface}`}
            value={`↓ ${formatBitRate(n.rx_bps, lng)} · ↑ ${formatBitRate(n.tx_bps, lng)}`}
          />
        ))}
        <Field
          label={t('metric.temperature')}
          value={stats.cpu.temp_c === null ? t('common.notAvailable') : formatTemperature(stats.cpu.temp_c, lng)}
        />
        {stats.gpus.map((g) => (
          <Field
            key={g.name}
            label={g.name}
            value={`${formatPercent(g.util_pct, lng)} · ${formatBytes(g.mem_used, lng)} / ${formatBytes(g.mem_total, lng)} · ${formatTemperature(g.temp_c, lng)}`}
          />
        ))}
        {stats.battery !== null && (
          <Field
            label={t('metric.battery')}
            value={`${formatPercent(stats.battery.pct, lng)}, ${stats.battery.on_ac ? t('node.charging') : t('node.onBattery')}${
              stats.battery.health_pct === null ? '' : ` · ${t('detail.health')} ${formatPercent(stats.battery.health_pct, lng)}`
            }`}
          />
        )}
        <Field label={t('metric.uptime')} value={formatUptime(stats.uptime_s, units)} />
      </dl>
    </section>
  );
}

const rangeKeys = Object.keys(ranges) as RangeKey[];

function History({ nodeId }: { nodeId: string }) {
  const { t, i18n } = useTranslation();
  const lng = i18n.language;
  const [range, setRange] = useState<RangeKey>('1h');
  const stats = useNodeStats(nodeId, range);
  const charts = useMemo(() => buildCharts(stats.data?.points ?? [], t, lng), [stats.data, t, lng]);

  return (
    <section className="history">
      <div className="history-head">
        <h3>{t('detail.history')}</h3>
        <div className="segmented" role="group" aria-label={t('detail.history')}>
          {rangeKeys.map((r) => (
            <button
              key={r}
              type="button"
              aria-pressed={r === range}
              onClick={() => {
                setRange(r);
              }}
            >
              {t(`range.${r}`)}
            </button>
          ))}
        </div>
      </div>
      {stats.isError && (
        <QueryError
          error={stats.error}
          onRetry={() => {
            void stats.refetch();
          }}
        />
      )}
      {stats.data !== undefined && stats.data.points.length === 0 && <p className="muted">{t('detail.noData')}</p>}
      {stats.data !== undefined && stats.data.points.length > 0 && (
        <Suspense fallback={<p className="muted">{t('common.loading')}</p>}>
          <div className="charts">
            {charts.map((c) => (
              <StatsChart key={c.title} {...c} />
            ))}
          </div>
        </Suspense>
      )}
    </section>
  );
}

type Translate = ReturnType<typeof useTranslation>['t'];

function buildCharts(points: Point[], t: Translate, lng: string) {
  const times = points.map((p) => Date.parse(p.ts) / 1000);
  const pct = (v: number) => formatPercent(v, lng);
  const bits = (v: number) => formatBitRate(v, lng);
  const temp = (v: number) => formatTemperature(v, lng);
  const col = (pick: (p: Point) => number | null) => points.map(pick);
  const one = (label: string, values: (number | null)[]): ChartSeries[] => [{ label, values, colorVar: '--chart-1' }];
  const hasValues = (values: (number | null)[]) => values.some((v) => v !== null);

  const charts: { title: string; times: number[]; series: ChartSeries[]; format: (v: number) => string; yRange?: [number, number] }[] = [
    { title: t('chart.cpu'), times, series: one(t('metric.cpu'), col((p) => p.cpu_pct)), format: pct, yRange: [0, 100] },
    {
      title: t('chart.memory'),
      times,
      series: one(t('metric.memory'), col((p) => usedPercent(p.mem_used, p.mem_total))),
      format: pct,
      yRange: [0, 100],
    },
    {
      title: t('chart.network'),
      times,
      series: [
        { label: t('metric.download'), values: col((p) => p.net_rx_bps), colorVar: '--chart-1' },
        { label: t('metric.upload'), values: col((p) => p.net_tx_bps), colorVar: '--chart-2' },
      ],
      format: bits,
    },
    { title: t('chart.disk'), times, series: one(t('metric.disk'), col((p) => p.disk_used_pct)), format: pct, yRange: [0, 100] },
  ];
  // Temperature and GPU only where the node can measure them at all.
  const temps = col((p) => p.cpu_temp_c);
  if (hasValues(temps)) charts.push({ title: t('chart.temperature'), times, series: one(t('metric.temperature'), temps), format: temp });
  const gpu = col((p) => p.gpu_util_pct);
  if (hasValues(gpu)) charts.push({ title: t('chart.gpu'), times, series: one(t('metric.gpu'), gpu), format: pct, yRange: [0, 100] });
  return charts;
}
