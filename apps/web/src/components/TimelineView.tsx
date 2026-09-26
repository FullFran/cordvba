import { symbolForLabel } from "../lib/labels";
import { splitTimeline } from "../lib/timeline";
import type { TimelinePoint } from "../types/environment";

export interface TimelineViewProps {
  points: TimelinePoint[];
  now: string;
  onSelectPoint?: (point: TimelinePoint) => void;
  /** An active scenario, shown as a distinct ◇ marker (issue #102). */
  scenarioPoint?: TimelinePoint;
}

function pointLabel(point: TimelinePoint): string {
  if (point.horizon_h !== undefined) {
    return `+${point.horizon_h}h`;
  }
  return point.at.slice(11, 16); // HH:MM
}

/**
 * Past observed points (●), now, future predicted points (◌ +1h/+3h/+6h)
 * and an optional scenario marker (◇), all distinguishable by text/shape,
 * not colour (issue #102). Selecting a point notifies the parent so it can
 * update the map and state panel.
 */
export function TimelineView({ points, now, onSelectPoint, scenarioPoint }: TimelineViewProps) {
  const { past, future } = splitTimeline(points, now);

  return (
    <ol className="timeline" aria-label="Timeline">
      {past.map((point) => (
        <li key={point.at}>
          <button type="button" onClick={() => onSelectPoint?.(point)}>
            <span aria-hidden="true">{symbolForLabel(point.label)}</span> {pointLabel(point)}
          </button>
        </li>
      ))}
      <li aria-current="time">now</li>
      {future.map((point) => (
        <li key={point.at}>
          <button type="button" onClick={() => onSelectPoint?.(point)}>
            <span aria-hidden="true">{symbolForLabel(point.label)}</span> {pointLabel(point)}
          </button>
        </li>
      ))}
      {scenarioPoint ? (
        <li>
          <button type="button" onClick={() => onSelectPoint?.(scenarioPoint)}>
            <span aria-hidden="true">{symbolForLabel("SIMULATED")}</span> scenario
          </button>
        </li>
      ) : null}
    </ol>
  );
}
