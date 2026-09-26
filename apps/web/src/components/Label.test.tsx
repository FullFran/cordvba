import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Label } from "./Label";

describe("Label", () => {
  it("renders the label text and its symbol for an observed value", () => {
    render(<Label label="OBSERVED" />);
    const el = screen.getByText(/OBSERVED/);
    expect(el).toBeInTheDocument();
    expect(el.textContent).toContain("●"); // ●
  });

  it("renders visibly different text/symbol per label so they never rely on colour alone", () => {
    const { container: observed } = render(<Label label="OBSERVED" />);
    const { container: predicted } = render(<Label label="PREDICTED" />);
    const { container: simulated } = render(<Label label="SIMULATED" />);

    expect(observed.textContent).not.toBe(predicted.textContent);
    expect(predicted.textContent).not.toBe(simulated.textContent);
    expect(observed.textContent).not.toBe(simulated.textContent);
  });
});
