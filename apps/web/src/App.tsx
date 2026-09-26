import { useEffect, useState } from "react";

import { getEnvironment, getSources, getTimeline, postSimulate } from "./api/client";
import { Footer } from "./components/Footer";
import { Header } from "./components/Header";
import { MapView } from "./components/MapView";
import { ProvenancePanel } from "./components/ProvenancePanel";
import { ScenarioPanel } from "./components/ScenarioPanel";
import { StatePanel } from "./components/StatePanel";
import { TimelineView } from "./components/TimelineView";
import type { EnvironmentResponse, SourcesResponse, TimelineResponse, Value } from "./types/environment";

const HOURS_BACK = 12;
const HORIZONS_H = [1, 3, 6];

export function App() {
  const [environment, setEnvironment] = useState<EnvironmentResponse | null>(null);
  const [timeline, setTimeline] = useState<TimelineResponse | null>(null);
  const [sources, setSources] = useState<SourcesResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selectedValue, setSelectedValue] = useState<Value | null>(null);
  const [highlightStationId, setHighlightStationId] = useState<string | undefined>(undefined);

  useEffect(() => {
    let cancelled = false;

    Promise.all([getEnvironment(), getTimeline(HOURS_BACK, HORIZONS_H), getSources()])
      .then(([environmentResponse, timelineResponse, sourcesResponse]) => {
        if (cancelled) return;
        setEnvironment(environmentResponse);
        setTimeline(timelineResponse);
        setSources(sourcesResponse);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : "failed to load the environment twin");
      });

    return () => {
      cancelled = true;
    };
  }, []);

  if (error) {
    return (
      <main className="app app--error">
        <Header />
        <p role="alert">Could not load CORDVBA: {error}</p>
      </main>
    );
  }

  if (!environment || !timeline || !sources) {
    return (
      <main className="app app--loading">
        <Header />
        <p>Loading Córdoba…</p>
      </main>
    );
  }

  const airQualitySeries = timeline.series.find((s) => s.variable === "air_quality_index");

  return (
    <div className="app">
      <Header />

      <MapView environment={environment} highlightStationId={highlightStationId} />

      {airQualitySeries ? (
        <TimelineView
          points={airQualitySeries.points}
          now={timeline.now}
          onSelectPoint={(point) => {
            setSelectedValue(point);
            setHighlightStationId(airQualitySeries.entity.id);
          }}
        />
      ) : null}

      <StatePanel environment={environment} onSelect={setSelectedValue} />

      <ScenarioPanel onSimulate={postSimulate} onSelectValue={setSelectedValue} />

      {selectedValue ? (
        <div role="dialog" aria-label="Provenance">
          <button type="button" onClick={() => setSelectedValue(null)}>
            Close
          </button>
          <ProvenancePanel value={selectedValue} />
        </div>
      ) : null}

      <Footer sources={sources.sources} />
    </div>
  );
}
