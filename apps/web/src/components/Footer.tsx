import { formatAge } from "../lib/age";
import type { Source } from "../types/environment";

export interface FooterProps {
  sources: Source[];
  now?: Date;
}

/**
 * Attribution for every data source and the map tiles, plus freshness per
 * source from GET /v1/sources (issue #102, AC-4).
 */
export function Footer({ sources, now }: FooterProps) {
  return (
    <footer className="page-footer">
      <ul>
        {sources.map((source) => (
          <li key={source.id}>
            {source.attribution} — updated {formatAge(source.last_ok, now)}
          </li>
        ))}
        <li>Map data © OpenStreetMap contributors</li>
      </ul>
    </footer>
  );
}
