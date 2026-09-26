import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Header } from "./Header";

describe("Header", () => {
  it("renders the title and the three explanatory lines from issue #102", () => {
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
