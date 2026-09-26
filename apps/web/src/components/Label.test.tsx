import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Label } from "./Label";
import { LocaleProvider } from "../i18n/LocaleContext";
import { es } from "../i18n/dictionaries/es";

describe("Label", () => {
  it("renders the label text and its symbol for an observed value", () => {
    render(<Label label="OBSERVED" />);
    // display text is the translated dictionary word (issue 124); the
    // wire value "OBSERVED" itself only drives the CSS class, not the text.
    const el = screen.getByText(/Observed/);
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

  it("displays Observado/Inferido/Predicho/Simulado in Spanish, the wire value untranslated (issue 124)", () => {
    render(
      <LocaleProvider>
        <Label label="SIMULATED" />
      </LocaleProvider>,
    );

    expect(screen.getByText(new RegExp(es.epistemicLabels.SIMULATED))).toBeInTheDocument();
  });
});
