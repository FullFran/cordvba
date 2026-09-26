import type {
  EnvironmentResponse,
  SimulateRequest,
  SimulateResponse,
  SourcesResponse,
  TimelineResponse,
} from "../types/environment";

/**
 * The web page never talks to eye or the twin directly (issue #102, AC-3):
 * it only ever calls this base URL, which api composes from all of them.
 */
function baseUrl(): string {
  return import.meta.env.VITE_API_BASE_URL ?? "/api";
}

function useFixtures(): boolean {
  return import.meta.env.VITE_USE_FIXTURES === "1";
}

async function requestJson<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${baseUrl()}${path}`, init);
  if (!response.ok) {
    throw new Error(`request to ${path} failed with status ${response.status}`);
  }
  return (await response.json()) as T;
}

export async function getEnvironment(): Promise<EnvironmentResponse> {
  if (useFixtures()) {
    const fixture = await import("@contracts/environment/v1/environment.example.json");
    return fixture.default as unknown as EnvironmentResponse;
  }
  return requestJson<EnvironmentResponse>("/v1/environment");
}

export async function getTimeline(
  hoursBack: number,
  horizons: number[],
): Promise<TimelineResponse> {
  if (useFixtures()) {
    const fixture = await import("@contracts/environment/v1/timeline.example.json");
    return fixture.default as unknown as TimelineResponse;
  }
  const horizonsParam = horizons.join(",");
  return requestJson<TimelineResponse>(
    `/v1/environment/timeline?hours_back=${hoursBack}&horizons=${horizonsParam}`,
  );
}

export async function getSources(): Promise<SourcesResponse> {
  if (useFixtures()) {
    const fixture = await import("@contracts/environment/v1/sources.example.json");
    return fixture.default as unknown as SourcesResponse;
  }
  return requestJson<SourcesResponse>("/v1/sources");
}

export async function postSimulate(request: SimulateRequest): Promise<SimulateResponse> {
  if (useFixtures()) {
    const fixture = await import("@contracts/environment/v1/simulate.response.example.json");
    return fixture.default as unknown as SimulateResponse;
  }
  return requestJson<SimulateResponse>("/v1/environment/simulate", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(request),
  });
}
