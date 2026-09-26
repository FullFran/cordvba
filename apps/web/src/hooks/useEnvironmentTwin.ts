import { useCallback, useEffect, useRef, useState } from "react";

import { getEnvironment, getSources, getTimeline } from "../api/client";
import type { EnvironmentResponse, SourcesResponse, TimelineResponse } from "../types/environment";

const HOURS_BACK = 12;
const HORIZONS_H = [1, 3, 6];

/**
 * Automatic retry backoff after a failed fetch (issue #119): 2s, 4s, 8s,
 * then stop and wait for the person to press Retry. Bounded so a source
 * that is genuinely down never turns into a silent request storm.
 */
export const AUTO_RETRY_DELAYS_MS = [2000, 4000, 8000];

export type ResourceState<T> =
  | { status: "loading" }
  | { status: "success"; data: T }
  | { status: "error"; message: string };

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : "the request failed for an unknown reason";
}

export interface EnvironmentTwin {
  environment: ResourceState<EnvironmentResponse>;
  timeline: ResourceState<TimelineResponse>;
  sources: ResourceState<SourcesResponse>;
  /** Re-fetches whichever of the three resources is not already loaded, without discarding the ones that succeeded. */
  retry: () => void;
}

/**
 * Fetches the three GET resources the environment twin page needs, each
 * independently: one endpoint being unreachable never blanks the page for
 * data that arrived fine (issue #119). A failed resource retries itself
 * automatically with backoff, and `retry()` lets the person force it.
 */
export function useEnvironmentTwin(): EnvironmentTwin {
  const [environment, setEnvironment] = useState<ResourceState<EnvironmentResponse>>({
    status: "loading",
  });
  const [timeline, setTimeline] = useState<ResourceState<TimelineResponse>>({ status: "loading" });
  const [sources, setSources] = useState<ResourceState<SourcesResponse>>({ status: "loading" });
  const [attempt, setAttempt] = useState(0);

  // Read via refs inside the retry-triggered effect below, so a resource
  // that already succeeded is never re-fetched (and never flashes back to
  // "loading") just because a sibling resource is being retried.
  const environmentRef = useRef(environment);
  environmentRef.current = environment;
  const timelineRef = useRef(timeline);
  timelineRef.current = timeline;
  const sourcesRef = useRef(sources);
  sourcesRef.current = sources;

  const load = useCallback(() => {
    if (environmentRef.current.status !== "success") {
      setEnvironment({ status: "loading" });
      getEnvironment()
        .then((data) => setEnvironment({ status: "success", data }))
        .catch((err: unknown) => setEnvironment({ status: "error", message: errorMessage(err) }));
    }

    if (timelineRef.current.status !== "success") {
      setTimeline({ status: "loading" });
      getTimeline(HOURS_BACK, HORIZONS_H)
        .then((data) => setTimeline({ status: "success", data }))
        .catch((err: unknown) => setTimeline({ status: "error", message: errorMessage(err) }));
    }

    if (sourcesRef.current.status !== "success") {
      setSources({ status: "loading" });
      getSources()
        .then((data) => setSources({ status: "success", data }))
        .catch((err: unknown) => setSources({ status: "error", message: errorMessage(err) }));
    }
  }, []);

  // eslint-disable-next-line react-hooks/exhaustive-deps -- `attempt` is the only intentional trigger; `load` is stable.
  useEffect(() => {
    load();
  }, [attempt, load]);

  useEffect(() => {
    const hasError =
      environment.status === "error" || timeline.status === "error" || sources.status === "error";
    if (!hasError) {
      return undefined;
    }

    const delay = AUTO_RETRY_DELAYS_MS[attempt];
    if (delay === undefined) {
      return undefined; // automatic retries exhausted; wait for a manual Retry
    }

    const timer = setTimeout(() => setAttempt((a) => a + 1), delay);
    return () => clearTimeout(timer);
  }, [environment.status, timeline.status, sources.status, attempt]);

  const retry = useCallback(() => setAttempt((a) => a + 1), []);

  return { environment, timeline, sources, retry };
}
