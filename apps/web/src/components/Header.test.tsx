import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { Header } from "./Header";
import { LocaleProvider } from "../i18n/LocaleContext";

describe("Header", () => {
  it("renders the title and the three explanatory lines from issue 102", () => {
    render(<Header />);

    expect(
      screen.getByRole("heading", { name: /CORDVBA.*Experimental digital model of Córdoba/ }),
    ).toBeInTheDocument();
    expect(screen.getByText(/collected and normalised by eye/i)).toBeInTheDocument();
    expect(
      screen.getByText(/reconstructs the city's current environmental state/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/whether it is observed, predicted or simulated/i),
    ).toBeInTheDocument();
  });
});

describe("Header: locale switch (issue 124)", () => {
  it("shows an ES | EN segmented control with aria-pressed reflecting the active locale (AC-5)", () => {
    render(
      <LocaleProvider>
        <Header />
      </LocaleProvider>,
    );

    const es = screen.getByRole("button", { name: "ES" });
    const en = screen.getByRole("button", { name: "EN" });
    expect(es).toHaveAttribute("aria-pressed", "true"); // Spanish default
    expect(en).toHaveAttribute("aria-pressed", "false");
  });

  it("switching to EN updates the header text without a reload, and is keyboard operable", async () => {
    render(
      <LocaleProvider>
        <Header />
      </LocaleProvider>,
    );

    await userEvent.tab(); // reaches the first tabbable control (ES button)
    await userEvent.tab(); // EN button
    await userEvent.keyboard("{Enter}");

    expect(
      screen.getByRole("heading", { name: /CORDVBA.*Experimental digital model of Córdoba/ }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "EN" })).toHaveAttribute("aria-pressed", "true");
  });
});
