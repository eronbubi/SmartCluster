import { useEffect, useRef } from 'react';
import uPlot from 'uplot';
import 'uplot/dist/uPlot.min.css';

export interface ChartSeries {
  label: string;
  values: (number | null)[];
  /** CSS custom property of the line color, e.g. "--chart-1". */
  colorVar: string;
}

interface Props {
  title: string;
  /** Unix timestamps in seconds. */
  times: number[];
  series: ChartSeries[];
  format: (value: number) => string;
  /** Fixed y range, e.g. [0, 100] for percentages. */
  yRange?: [number, number];
  height?: number;
}

function cssVar(el: Element, name: string): string {
  return getComputedStyle(el).getPropertyValue(name).trim() || 'currentColor';
}

/** A time series chart (uPlot). Null values leave gaps instead of drawing 0. */
export function StatsChart({ title, times, series, format, yRange, height = 180 }: Props) {
  const container = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = container.current;
    if (el === null) return;
    const axisColor = cssVar(el, '--text-muted');
    const gridColor = cssVar(el, '--border');
    const opts: uPlot.Options = {
      width: el.clientWidth || 600,
      height,
      legend: { show: series.length > 1 },
      cursor: { drag: { x: false, y: false } },
      scales: { y: yRange === undefined ? {} : { range: yRange } },
      axes: [
        { stroke: axisColor, grid: { stroke: gridColor } },
        { stroke: axisColor, grid: { stroke: gridColor }, size: 70, values: (_u, ticks) => ticks.map(format) },
      ],
      series: [
        {},
        ...series.map((s) => ({
          label: s.label,
          stroke: cssVar(el, s.colorVar),
          width: 1.5,
          spanGaps: false,
          value: (_u: uPlot, v: number | null) => (v === null ? '–' : format(v)),
        })),
      ],
    };
    const chart = new uPlot(opts, [times, ...series.map((s) => s.values)], el);
    const resize = new ResizeObserver(() => {
      chart.setSize({ width: el.clientWidth, height });
    });
    resize.observe(el);
    return () => {
      resize.disconnect();
      chart.destroy();
    };
  }, [times, series, format, yRange, height]);

  return (
    <figure className="chart">
      <figcaption>{title}</figcaption>
      <div ref={container} className="chart-canvas" />
    </figure>
  );
}
