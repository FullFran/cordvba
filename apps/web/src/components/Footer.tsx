import { useLocale } from "../i18n/LocaleContext";
import { formatAge } from "../lib/age";
import { formatSourceAttribution } from "../lib/attribution";
import type { Source } from "../types/environment";

export interface FooterProps {
  sources: Source[];
  now?: Date;
}

/**
 * Attribution for every data source and the map tiles, plus freshness per
 * source from GET /v1/sources (issue #102, AC-4). The map moved from
 * Leaflet/OSM raster tiles to MapLibre/OpenFreeMap vector tiles (issue
 * #119), so the required attribution changed with it: "OpenFreeMap ©
 * OpenMapTiles Data from OpenStreetMap" replaces the old bare "Map data ©
 * OpenStreetMap contributors" line. The per-source line is built
 * client-side from id/publisher/licence (`formatSourceAttribution`), not
 * the API's own pre-formatted, English-only `attribution` string (issue
 * #124, maintainer review: it showed English inside the Spanish UI).
 */
export function Footer({ sources, now }: FooterProps) {
  const { locale, t } = useLocale();

  return (
    <footer className="page-footer">
      <ul>
        {sources.map((source) => (
          <li key={source.id}>
            {formatSourceAttribution(source, locale)} — {t.footer.updatedPrefix}{" "}
            {formatAge(source.last_ok, now, locale)}
          </li>
        ))}
        <li>{t.footer.tileAttribution}</li>
      </ul>
    </footer>
  );
}
