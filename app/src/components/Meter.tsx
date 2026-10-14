/** A labelled bar for a value in 0..100. Null shows an empty bar. */
export function Meter({ label, percent, text }: { label: string; percent: number | null; text: string }) {
  const level = percent === null ? 'none' : percent >= 90 ? 'high' : percent >= 75 ? 'mid' : 'low';
  return (
    <div className="meter">
      <div className="meter-head">
        <span className="meter-label">{label}</span>
        <span className="meter-text">{text}</span>
      </div>
      <div
        className={`meter-bar meter-${level}`}
        role="meter"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={percent === null ? undefined : Math.round(percent)}
      >
        <div className="meter-fill" style={{ width: `${percent ?? 0}%` }} />
      </div>
    </div>
  );
}
