import { afterEach, describe, expect, it, vi } from "vitest";

import environmentFixture from "@contracts/environment/v1/environment.example.json";
import sourcesFixture from "@contracts/environment/v1/sources.example.json";
import timelineFixture from "@contracts/environment/v1/timeline.example.json";

import { getEnvironment, getSources, getTimeline, postSimulate } from "./client";

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("api client in fixture mode (VITE_USE_FIXTURES=1)", () => {
  it("serves the environment fixture without any network request", async () => {
    vi.stubEnv("VITE_USE_FIXTURES", "1");
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);

    const result = await getEnvironment();

    expect(result).toEqual(environmentFixture);
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("serves the timeline fixture without any network request", async () => {
    vi.stubEnv("VITE_USE_FIXTURES", "1");
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);

    const result = await getTimeline(12, [1, 3, 6]);

    expect(result).toEqual(timelineFixture);
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("serves the sources fixture without any network request", async () => {
    vi.stubEnv("VITE_USE_FIXTURES", "1");
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);

    const result = await getSources();

    expect(result).toEqual(sourcesFixture);
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("serves the simulate-response fixture for any scenario, without any network request", async () => {
    vi.stubEnv("VITE_USE_FIXTURES", "1");
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);

    const result = await postSimulate({
      temperature_delta_c: 3,
      humidity_delta_pct: 10,
      wind_factor: 0.5,
    });

    expect(result.simulated.air_temperature.label).toBe("SIMULATED");
    expect(fetchSpy).not.toHaveBeenCalled();
  });
});

describe("api client against the API (VITE_USE_FIXTURES unset), with fetch mocked", () => {
  it("requests GET /v1/environment under the configured base URL", async () => {
    vi.stubEnv("VITE_API_BASE_URL", "/api");
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => environmentFixture,
    });
    vi.stubGlobal("fetch", fetchSpy);

    const result = await getEnvironment();

    expect(fetchSpy).toHaveBeenCalledWith("/api/v1/environment", undefined);
    expect(result).toEqual(environmentFixture);
  });

  it("requests GET /v1/environment/timeline with hours_back and horizons as query params", async () => {
    vi.stubEnv("VITE_API_BASE_URL", "/api");
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => timelineFixture,
    });
    vi.stubGlobal("fetch", fetchSpy);

    await getTimeline(12, [1, 3, 6]);

    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/v1/environment/timeline?hours_back=12&horizons=1,3,6",
      undefined,
    );
  });

  it("POSTs the scenario JSON to /v1/environment/simulate", async () => {
    vi.stubEnv("VITE_API_BASE_URL", "/api");
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({}),
    });
    vi.stubGlobal("fetch", fetchSpy);

    const request = { temperature_delta_c: 3, humidity_delta_pct: 10, wind_factor: 0.5 };
    await postSimulate(request);

    expect(fetchSpy).toHaveBeenCalledWith("/api/v1/environment/simulate", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(request),
    });
  });

  it("throws instead of returning a partial result when the API responds with an error status", async () => {
    vi.stubEnv("VITE_API_BASE_URL", "/api");
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: false, status: 500, json: async () => ({}) }),
    );

    await expect(getEnvironment()).rejects.toThrow(/500/);
  });

  it("defaults the base URL to /api when VITE_API_BASE_URL is not set", async () => {
    const fetchSpy = vi.fn().mockResolvedValue({ ok: true, json: async () => environmentFixture });
    vi.stubGlobal("fetch", fetchSpy);

    await getEnvironment();

    expect(fetchSpy).toHaveBeenCalledWith("/api/v1/environment", undefined);
  });
});
