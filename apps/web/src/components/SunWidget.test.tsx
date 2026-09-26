import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { SunWidget } from "./SunWidget";
import { LocaleProvider } from "../i18n/LocaleContext";

// Fixed UTC instants (Europe/Madrid is CEST, UTC+2, in late September) so
// this test never depends on the machine's own local timezone.
const SUNRISE = new Date("2026-09-26T05:57:00Z"); // 07:57 Europe/Madrid
const SOLAR_NOON = new Date("2026-09-26T11:11:00Z"); // 13:11 Europe/Madrid
const SUNSET = new Date("2026-09-26T18:24:00Z"); // 20:24 Europe/Madrid

describe("SunWidget (issue #139: sunrise, solar noon, sunset, current altitude)", () => {
  it("shows sunrise, solar noon and sunset in Córdoba's own local time", () => {
    render(<SunWidget sunrise={SUNRISE} solarNoon={SOLAR_NOON} sunset={SUNSET} altitudeDeg={42.3} />);

    expect(screen.getByText("07:57")).toBeInTheDocument();
    expect(screen.getByText("13:11")).toBeInTheDocument();
    expect(screen.getByText("20:24")).toBeInTheDocument();
  });

  it("shows the current altitude as a rounded degree value, never a raw 'deg' unit code", () => {
    render(<SunWidget sunrise={SUNRISE} solarNoon={SOLAR_NOON} sunset={SUNSET} altitudeDeg={42.3} />);

    expect(screen.getByText("42°")).toBeInTheDocument();
    expect(screen.queryByText(/deg/)).not.toBeInTheDocument();
  });

  it("shows an em dash, not 'Invalid Date', for a polar day/night with no sunrise or sunset", () => {
    render(<SunWidget sunrise={null} solarNoon={SOLAR_NOON} sunset={null} altitudeDeg={5} />);

    expect(screen.getAllByText("—").length).toBeGreaterThanOrEqual(2);
    expect(screen.queryByText(/invalid/i)).not.toBeInTheDocument();
  });

  it("has an accessible label naming the sun widget", () => {
    render(<SunWidget sunrise={SUNRISE} solarNoon={SOLAR_NOON} sunset={SUNSET} altitudeDeg={42.3} />);
    expect(screen.getByLabelText("Sun")).toBeInTheDocument();
  });

  it("renders every label in Spanish when given the es locale (issue 124)", () => {
    render(
      <LocaleProvider>
        <SunWidget sunrise={SUNRISE} solarNoon={SOLAR_NOON} sunset={SUNSET} altitudeDeg={42.3} />
      </LocaleProvider>,
    );
    expect(screen.getByLabelText("Sol")).toBeInTheDocument();
    expect(screen.getByText("Amanecer")).toBeInTheDocument();
    expect(screen.getByText("Mediodía solar")).toBeInTheDocument();
    expect(screen.getByText("Atardecer")).toBeInTheDocument();
  });

  it("compacts into a single flowing row, not four stacked block lines (parent review: the dock must never clip)", () => {
    const { container } = render(
      <SunWidget sunrise={SUNRISE} solarNoon={SOLAR_NOON} sunset={SUNSET} altitudeDeg={42.3} />,
    );

    // Four inline items, none of them a block-level <p> (the old one-per-line shape).
    expect(container.querySelectorAll(".sun-widget__item")).toHaveLength(4);
    expect(container.querySelectorAll(".sun-widget p")).toHaveLength(0);
  });
});
