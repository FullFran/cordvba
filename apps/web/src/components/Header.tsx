import { useLocale } from "../i18n/LocaleContext";
import type { Locale } from "../i18n/types";

const LOCALES: readonly Locale[] = ["es", "en"];

function LocaleSwitch() {
  const { locale, setLocale, t } = useLocale();

  return (
    <div className="locale-switch" role="group" aria-label={t.localeSwitch.groupLabel}>
      {LOCALES.map((candidate) => (
        <button
          key={candidate}
          type="button"
          className="locale-switch__option"
          aria-pressed={locale === candidate}
          onClick={() => setLocale(candidate)}
        >
          {t.localeSwitch[candidate]}
        </button>
      ))}
    </div>
  );
}

export function Header() {
  const { t } = useLocale();

  return (
    <header className="page-header">
      <div className="page-header__row">
        <h1>{t.header.title}</h1>
        <LocaleSwitch />
      </div>
      <p>{t.header.line1}</p>
      <p>{t.header.line2}</p>
      <p>{t.header.line3}</p>
    </header>
  );
}
