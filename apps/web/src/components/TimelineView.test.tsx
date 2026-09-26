import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import timelineFixture from "@contracts/environment/v1/timeline.example.json";

import { TimelineView } from "./TimelineView";
import type { TimelineResponse } from "../types/environment";

const timeline = timelineFixture as unknown as TimelineResponse;
const airQualitySeries = timeline.series.find((s) => s.variable === "air_quality_index");
if (!airQualitySeries) {
  throw new Error("fixture is missing the air_quality_index series");
}

const [firstAirQualityPoint] = airQualitySeries.points;
if (!firstAirQualityPoint) {
  throw new Error("fixture air_quality_index series has no points");
}

describe("TimelineView", () => {
  it("renders past points before now and future points after now, in order", () => {
    render(
      <TimelineView points={airQualitySeries.points} now={timeline.now} onSelectPoint={vi.fn()} />,
    );

    const list = screen.getByRole("list", { name: /timeline/i });
    const items = within(list).getAllByRole("listitem");
    const texts = items.map((item) => item.textContent ?? "");

    const pastIndex = texts.findIndex((t) => t.includes("08:00"));
    const nowIndex = texts.findIndex((t) => /now/i.test(t));
    const futureIndex = texts.findIndex((t) => t.includes("+1"));

    expect(pastIndex).toBeGreaterThanOrEqual(0);
    expect(nowIndex).toBeGreaterThan(pastIndex);
    expect(futureIndex).toBeGreaterThan(nowIndex);
  });

  it("marks past points with the observed symbol and future points with the predicted symbol", () => {
    render(
      <TimelineView points={airQualitySeries.points} now={timeline.now} onSelectPoint={vi.fn()} />,
    );

    expect(screen.getAllByText(/●/).length).toBeGreaterThan(0); // ● observed
    expect(screen.getAllByText(/◌/).length).toBeGreaterThan(0); // ◌ predicted
  });

  it("shows a scenario marker (◇) distinct from past/future when a scenario point is given", () => {
    const scenarioPoint = { ...firstAirQualityPoint, label: "SIMULATED" as const };
    render(
      <TimelineView
        points={airQualitySeries.points}
        now={timeline.now}
        scenarioPoint={scenarioPoint}
        onSelectPoint={vi.fn()}
      />,
    );

    expect(screen.getByText(/◇/)).toBeInTheDocument(); // ◇ scenario
  });

  it("calls onSelectPoint with the clicked point", async () => {
    const onSelectPoint = vi.fn();
    render(
      <TimelineView
        points={airQualitySeries.points}
        now={timeline.now}
        onSelectPoint={onSelectPoint}
      />,
    );

    const list = screen.getByRole("list", { name: /timeline/i });
    const [firstPointButton] = within(list).getAllByRole("button");
    if (!firstPointButton) {
      throw new Error("timeline rendered no point buttons");
    }
    await userEvent.click(firstPointButton);

    expect(onSelectPoint).toHaveBeenCalledWith(firstAirQualityPoint);
  });

  it("shows the clock time as the primary label and the forecast horizon as secondary text (polish: the old '+1h before now' confusion)", () => {
    render(
      <TimelineView points={airQualitySeries.points} now={timeline.now} onSelectPoint={vi.fn()} />,
    );

    const list = screen.getByRole("list", { name: /timeline/i });
    const predictedPoint = airQualitySeries.points.find((p) => p.horizon_h === 1);
    if (!predictedPoint) {
      throw new Error("fixture is missing a +1h predicted point");
    }
    const clockTime = predictedPoint.at.slice(11, 16);
    const item = within(list)
      .getAllByRole("listitem")
      .find((li) => li.textContent?.includes(clockTime));

    expect(item?.textContent).toContain(clockTime);
    expect(item?.textContent).toContain("+1h");
  });
});
