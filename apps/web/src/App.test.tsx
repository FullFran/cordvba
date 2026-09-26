import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import environmentFixture from "@contracts/environment/v1/environment.example.json";
import sourcesFixture from "@contracts/environment/v1/sources.example.json";
import timelineFixture from "@contracts/environment/v1/timeline.example.json";

// MapView owns its own MapLibre integration and is covered by its own test
// file (with maplibre-gl mocked there); App only needs to know it renders
// somewhere once environment data exists.
vi.mock("./components/MapView", () => ({
  MapView: () => <div data-testid="map-view-stub" />,
}));

import { App } from "./App";
import * as apiClient from "./api/client";

function neverResolves<T>(): Promise<T> {
  return new Promise<T>(() => {
    /* deliberately never settles, to exercise the loading state */
  });
}

describe("App: resilient loading and error states (issue 119)", () => {
  beforeEach(() => {
    vi.spyOn(apiClient, "getEnvironment");
    vi.spyOn(apiClient, "getTimeline");
    vi.spyOn(apiClient, "getSources");
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  it("shows a loading skeleton, not a blank page, while data is in flight", () => {
    vi.mocked(apiClient.getEnvironment).mockReturnValue(neverResolves());
    vi.mocked(apiClient.getTimeline).mockReturnValue(neverResolves());
    vi.mocked(apiClient.getSources).mockReturnValue(neverResolves());

    render(<App />);

    // The page identity is always visible, even before any data arrives.
    expect(
      screen.getByRole("heading", { name: /CORDVBA.*Experimental digital model of Córdoba/ }),
    ).toBeInTheDocument();
    expect(screen.getByRole("status", { name: /loading/i })).toBeInTheDocument();
  });

  it("renders the full page once every resource resolves", async () => {
    vi.mocked(apiClient.getEnvironment).mockResolvedValue(environmentFixture as never);
    vi.mocked(apiClient.getTimeline).mockResolvedValue(timelineFixture as never);
    vi.mocked(apiClient.getSources).mockResolvedValue(sourcesFixture as never);

    render(<App />);

    expect(await screen.findByTestId("map-view-stub")).toBeInTheDocument();
    expect(screen.getByLabelText(/current state/i)).toBeInTheDocument();
    expect(screen.queryByRole("status", { name: /loading/i })).not.toBeInTheDocument();
    expect(screen.queryByText(/unreachable/i)).not.toBeInTheDocument();
  });

  it("shows a styled network-error panel with a reason and a Retry button, never a raw error line", async () => {
    vi.mocked(apiClient.getEnvironment).mockRejectedValue(new Error("NetworkError: fetch failed"));
    vi.mocked(apiClient.getTimeline).mockResolvedValue(timelineFixture as never);
    vi.mocked(apiClient.getSources).mockResolvedValue(sourcesFixture as never);

    render(<App />);

    const panel = await screen.findByRole("alert");
    expect(within(panel).getByText(/live data is unreachable right now/i)).toBeInTheDocument();
    expect(within(panel).getByText(/networkerror: fetch failed/i)).toBeInTheDocument();
    expect(within(panel).getByRole("button", { name: /retry/i })).toBeInTheDocument();

    // never the old bare "Could not load CORDVBA: <message>" line
    expect(screen.queryByText(/^Could not load CORDVBA/)).not.toBeInTheDocument();
  });

  it("retries the failed resource when Retry is clicked, and renders normally once it succeeds", async () => {
    vi.mocked(apiClient.getEnvironment)
      .mockRejectedValueOnce(new Error("NetworkError: fetch failed"))
      .mockResolvedValueOnce(environmentFixture as never);
    vi.mocked(apiClient.getTimeline).mockResolvedValue(timelineFixture as never);
    vi.mocked(apiClient.getSources).mockResolvedValue(sourcesFixture as never);

    render(<App />);

    const retryButton = await screen.findByRole("button", { name: /retry/i });
    await userEvent.click(retryButton);

    expect(await screen.findByTestId("map-view-stub")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(apiClient.getEnvironment).toHaveBeenCalledTimes(2);
  });

  it("shows what arrived and marks only the failed resource unavailable, instead of blocking the whole page", async () => {
    vi.mocked(apiClient.getEnvironment).mockResolvedValue(environmentFixture as never);
    vi.mocked(apiClient.getTimeline).mockResolvedValue(timelineFixture as never);
    vi.mocked(apiClient.getSources).mockRejectedValue(new Error("NetworkError: fetch failed"));

    render(<App />);

    // The map and state, which only need `environment`, render normally.
    expect(await screen.findByTestId("map-view-stub")).toBeInTheDocument();
    expect(screen.getByLabelText(/current state/i)).toBeInTheDocument();

    // Only the sources-dependent footer is marked unavailable.
    expect(screen.getByText(/attribution unavailable/i)).toBeInTheDocument();
  });

  it("automatically retries a failed resource with backoff, without the user clicking anything", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });

    vi.mocked(apiClient.getEnvironment)
      .mockRejectedValueOnce(new Error("NetworkError: fetch failed"))
      .mockResolvedValueOnce(environmentFixture as never);
    vi.mocked(apiClient.getTimeline).mockResolvedValue(timelineFixture as never);
    vi.mocked(apiClient.getSources).mockResolvedValue(sourcesFixture as never);

    render(<App />);

    await vi.waitFor(() => expect(apiClient.getEnvironment).toHaveBeenCalledTimes(1));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });

    await vi.waitFor(() => expect(apiClient.getEnvironment).toHaveBeenCalledTimes(2));
  });
});
