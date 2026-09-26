/**
 * Placeholder shapes shown while the environment twin's data is in flight
 * (issue #119): a blank page reads as broken, so this stands in for the
 * map, timeline and panels until they have something real to show. The
 * shimmer animation is decorative only and is frozen under
 * `prefers-reduced-motion` by `.skeleton__block` in styles.css.
 */
export function Skeleton() {
  return (
    <div className="skeleton" aria-hidden="true">
      <div className="skeleton__block skeleton__block--map" />
      <div className="skeleton__block skeleton__block--timeline" />
      <div className="skeleton__block skeleton__block--panel" />
      <div className="skeleton__block skeleton__block--panel" />
    </div>
  );
}

export interface LoadingStatusProps {
  label: string;
}

/** The accessible half of the loading state: an SR/live-region announcement the skeleton itself can't give. */
export function LoadingStatus({ label }: LoadingStatusProps) {
  return (
    <p role="status" aria-label={label} className="visually-hidden">
      {label}
    </p>
  );
}
