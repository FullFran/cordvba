import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

import { LocaleProvider, useLocale } from "./LocaleContext";
import { LOCALE_STORAGE_KEY } from "./storage";

function Probe() {
  const { locale, setLocale, t } = useLocale();
  return (
    <div>
      <p data-testid="locale">{locale}</p>
      <p data-testid="title">{t.header.title}</p>
      <button type="button" onClick={() => setLocale("en")}>
        switch to en
      </button>
    </div>
  );
}

describe("LocaleProvider (issue 124)", () => {
  afterEach(() => {
    window.localStorage.clear();
    document.documentElement.lang = "";
  });

  it("defaults to Spanish when nothing is stored (AC-3)", () => {
    render(
      <LocaleProvider>
        <Probe />
      </LocaleProvider>,
    );

    expect(screen.getByTestId("locale")).toHaveTextContent("es");
    expect(screen.getByTestId("title")).toHaveTextContent("Modelo digital experimental");
    expect(document.documentElement.lang).toBe("es");
  });

  it("honours a locale the host site already stored under ff_locale (AC-3)", () => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, "en");

    render(
      <LocaleProvider>
        <Probe />
      </LocaleProvider>,
    );

    expect(screen.getByTestId("locale")).toHaveTextContent("en");
    expect(screen.getByTestId("title")).toHaveTextContent("Experimental digital model");
  });

  it("switching locale updates every consumer, sets lang, and persists to ff_locale (AC-2)", async () => {
    render(
      <LocaleProvider>
        <Probe />
      </LocaleProvider>,
    );

    await userEvent.click(screen.getByRole("button", { name: /switch to en/i }));

    expect(screen.getByTestId("locale")).toHaveTextContent("en");
    expect(document.documentElement.lang).toBe("en");
    expect(window.localStorage.getItem(LOCALE_STORAGE_KEY)).toBe("en");
  });

  it("useLocale() outside a provider still works, defaulting to English, rather than throwing", () => {
    render(<Probe />);
    expect(screen.getByTestId("locale")).toHaveTextContent("en");
  });
});
