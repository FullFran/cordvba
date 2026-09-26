import { useEffect, useState } from "react";

import { postSimulate } from "./api/client";
import { Footer } from "./components/Footer";
import { Header } from "./components/Header";
import { MapView } from "./components/MapView";
import { NetworkErrorPanel } from "./components/NetworkErrorPanel";
import { ProvenancePanel } from "./components/ProvenancePanel";
import { ScenarioPanel } from "./components/ScenarioPanel";
import { LoadingStatus, Skeleton } from "./components/Skeleton";
import { StatePanel } from "./components/StatePanel";
import { SunWidget } from "./components/SunWidget";
import { TimelineView } from "./components/TimelineView";
import { useEnvironmentTwin } from "./hooks/useEnvironmentTwin";
import { useLocale } from "./i18n/LocaleContext";
import { getSunPosition, getSunTimes } from "./lib/sun";
import type { Value } from "./types/environment";

/**
 * Full-bleed map with a restrained HUD over it (issue #119, maintainer
 * review): the map IS the page, not "a map in a box above a grid of
 * cards". `.app__hud-*` panels are opaque token surfaces
 * (`--color-bg-elevated` + a subtle border) positioned as an overlay on
 * desktop and stacked in normal document flow on mobile; see
 * `.app__hud` in styles.css for the breakpoint.
 */
export function App() {
  const { environment, timeline, sources, retry } = useEnvironmentTwin();
  const { t } = useLocale();
  const [selectedValue, setSelectedValue] = useState<Value | null>(null);
  const [highlightStationId, setHighlightStationId] = useState<string | undefined>(undefined);
  const [scenarioOpen, setScenarioOpen] = useState(false);
  /**
   * The timeline's selected time (issue #139 — "Córdoba bajo el sol"):
   * drives the sun widget, the map's sun-driven lighting/palette and its
   * building-shadow layer. Defaults to the environment payload's own
   * `generated_at` (the data's clock, so a fixture-mode demo and its
   * screenshots stay reproducible) rather than the visitor's wall clock,
   * until a timeline point is explicitly selected below.
   */
  const [selectedTime, setSelectedTime] = useState<Date>(() => new Date());
  useEffect(() => {
    if (environment.status === "success") {
      setSelectedTime(new Date(environment.data.generated_at));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only the loading->success transition should reset this; a value re-render must not fight the visitor's own timeline selection.
  }, [environment.status]);

  const allLoading =
    environment.status === "loading" && timeline.status === "loading" && sources.status === "loading";

  const airQualitySeries =
    timeline.status === "success"
      ? timeline.data.series.find((s) => s.variable === "air_quality_index")
      : undefined;

  return (
    <div className="app">
      <div className="app__stage">
        {allLoading ? (
          <Skeleton />
        ) : environment.status === "success" ? (
          <MapView
            environment={environment.data}
            highlightStationId={highlightStationId}
            sun={getSunPosition(selectedTime, environment.data.place.lat, environment.data.place.lon)}
          />
        ) : environment.status === "error" ? (
          <NetworkErrorPanel reason={environment.message} onRetry={retry} />
        ) : (
          <Skeleton />
        )}
      </div>

      {allLoading ? <LoadingStatus label={t.loading.label} /> : null}

      <div className="app__hud">
        <div className="app__panel app__panel--header">
          <Header />
        </div>

        {!allLoading ? (
          <div className="app__panel app__panel--state">
            {environment.status === "success" ? (
              <>
                <StatePanel environment={environment.data} onSelect={setSelectedValue} />
                <SunWidget
                  {...getSunTimes(selectedTime, environment.data.place.lat, environment.data.place.lon)}
                  altitudeDeg={
                    getSunPosition(selectedTime, environment.data.place.lat, environment.data.place.lon).altitudeDeg
                  }
                />
              </>
            ) : null}

            <button
              type="button"
              className="app__scenario-toggle"
              aria-expanded={scenarioOpen}
              onClick={() => setScenarioOpen((open) => !open)}
            >
              {t.scenario.ariaLabel}
            </button>
            {scenarioOpen ? (
              <ScenarioPanel onSimulate={postSimulate} onSelectValue={setSelectedValue} />
            ) : null}

            {/* Attribution lives at the bottom of the state dock, not its
                own corner (issue #119, maintainer review): a dedicated
                bottom-left footer panel collided with the map's own
                bottom-left legend once the wind/caption merge made that
                legend taller. */}
            {sources.status === "success" ? (
              <Footer sources={sources.data.sources} />
            ) : sources.status === "error" ? (
              <p className="unavailable-notice">
                {t.unavailable.attributionPrefix} {sources.message}
              </p>
            ) : null}
          </div>
        ) : null}

        <div className="app__panel app__panel--scrubber">
          {timeline.status === "success" && airQualitySeries ? (
            <TimelineView
              points={airQualitySeries.points}
              now={timeline.data.now}
              onSelectPoint={(point) => {
                setSelectedValue(point);
                setHighlightStationId(airQualitySeries.entity.id);
                setSelectedTime(new Date(point.at));
              }}
            />
          ) : timeline.status === "error" ? (
            <p className="unavailable-notice">
              {t.unavailable.timelinePrefix} {timeline.message}
            </p>
          ) : null}
        </div>
      </div>

      {selectedValue ? (
        <div className="app__dialog-backdrop">
          <div role="dialog" aria-label={t.provenance.dialogLabel} className="app__dialog">
            <button type="button" onClick={() => setSelectedValue(null)}>
              {t.provenance.close}
            </button>
            <ProvenancePanel value={selectedValue} />
          </div>
        </div>
      ) : null}
    </div>
  );
}
