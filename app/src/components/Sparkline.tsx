/** Points of a polyline for values in 0..100 inside width x height. */
export function sparklinePoints(values: readonly number[], width: number, height: number): string {
  if (values.length < 2) return '';
  const step = width / (values.length - 1);
  return values
    .map((v, i) => {
      const y = height - (Math.min(100, Math.max(0, v)) / 100) * height;
      return `${(i * step).toFixed(1)},${y.toFixed(1)}`;
    })
    .join(' ');
}

/** Small trend line of the last live samples; decorative, the number is shown next to it. */
export function Sparkline({ values, width = 120, height = 28 }: { values: readonly number[]; width?: number; height?: number }) {
  const points = sparklinePoints(values, width, height);
  return (
    <svg className="sparkline" width={width} height={height} viewBox={`0 0 ${width} ${height}`} aria-hidden="true">
      {points !== '' && <polyline points={points} fill="none" strokeWidth="1.5" />}
    </svg>
  );
}
