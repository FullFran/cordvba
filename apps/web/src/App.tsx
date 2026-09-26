import { useState } from "react";

import { postSimulate } from "./api/client";
import { Footer } from "./components/Footer";
import { Header } from "./components/Header";
import { MapView } from "./components/MapView";
import { NetworkErrorPanel } from "./components/NetworkErrorPanel";
import { ProvenancePanel } from "./components/ProvenancePanel";
import { ScenarioPanel } from "./components/ScenarioPanel";
import { LoadingStatus, Skeleton } from "./components/Skeleton";
import { StatePanel } from "./components/StatePanel";
import { TimelineView } from "./components/TimelineView";
import { useEnvironmentTwin } from "./hooks/useEnvironmentTwin";
import { useLocale } from "./i18n/LocaleContext";
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
          <MapView environment={environment.data} highlightStationId={highlightStationId} />
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
              <StatePanel environment={environment.data} onSelect={setSelectedValue} />
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
              }}
            />
          ) : timeline.status === "error" ? (
            <p className="unavailable-notice">
              {t.unavailable.timelinePrefix} {timeline.message}
            </p>
          ) : null}
        </div>

        {!allLoading ? (
          <div className="app__panel app__panel--footer">
            {sources.status === "success" ? (
              <Footer sources={sources.data.sources} />
            ) : sources.status === "error" ? (
              <p className="unavailable-notice">
                {t.unavailable.attributionPrefix} {sources.message}
              </p>
            ) : null}
          </div>
        ) : null}
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
