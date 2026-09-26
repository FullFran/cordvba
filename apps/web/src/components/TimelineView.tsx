import { useLocale } from "../i18n/LocaleContext";
import { symbolForLabel } from "../lib/labels";
import { pointHorizonLabel, pointLabel, splitTimeline, textureForLabel } from "../lib/timeline";
import type { TimelinePoint } from "../types/environment";

export interface TimelineViewProps {
  points: TimelinePoint[];
  now: string;
  onSelectPoint?: (point: TimelinePoint) => void;
  /** An active scenario, shown as a distinct ◇ marker (issue #102). */
  scenarioPoint?: TimelinePoint;
}

/**
 * Past observed points (●), now, future predicted points (◌ +1h/+3h/+6h)
 * and an optional scenario marker (◇), all distinguishable by text/shape,
 * not colour (issue #102). Selecting a point notifies the parent so it can
 * update the map and state panel.
 */
export function TimelineView({ points, now, onSelectPoint, scenarioPoint }: TimelineViewProps) {
  const { t } = useLocale();
  const { past, future } = splitTimeline(points, now);

  return (
    <ol className="timeline" aria-label={t.timeline.ariaLabel}>
      {past.map((point) => (
        <li key={point.at}>
          <button
            type="button"
            className={`timeline__point timeline__point--${textureForLabel(point.label)}`}
            onClick={() => onSelectPoint?.(point)}
          >
            <span aria-hidden="true">{symbolForLabel(point.label)}</span> {pointLabel(point)}
            {pointHorizonLabel(point) ? (
              <span className="timeline__point-horizon"> ({pointHorizonLabel(point)})</span>
            ) : null}
          </button>
        </li>
      ))}
      <li aria-current="time">{t.timeline.now}</li>
      {future.map((point) => (
        <li key={point.at}>
          <button
            type="button"
            className={`timeline__point timeline__point--${textureForLabel(point.label)}`}
            onClick={() => onSelectPoint?.(point)}
          >
            <span aria-hidden="true">{symbolForLabel(point.label)}</span> {pointLabel(point)}
            {pointHorizonLabel(point) ? (
              <span className="timeline__point-horizon"> ({pointHorizonLabel(point)})</span>
            ) : null}
          </button>
        </li>
      ))}
      {scenarioPoint ? (
        <li>
          <button
            type="button"
            className={`timeline__point timeline__point--${textureForLabel("SIMULATED")}`}
            onClick={() => onSelectPoint?.(scenarioPoint)}
          >
            <span aria-hidden="true">{symbolForLabel("SIMULATED")}</span> {t.timeline.scenario}
          </button>
        </li>
      ) : null}
    </ol>
  );
}
