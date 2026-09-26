import { useLocale } from "../i18n/LocaleContext";

export interface NetworkErrorPanelProps {
  /** The raw error message (e.g. "NetworkError: fetch failed"), shown as the technical reason. */
  reason: string;
  onRetry: () => void;
}

/**
 * Shown in place of a resource that failed to load (issue #119): a plain
 * "Could not load CORDVBA: <message>" line reads as broken and offers no
 * way forward. This names the situation honestly, keeps the technical
 * reason available but visually secondary, and offers a way to try again
 * (a manual Retry; `useEnvironmentTwin` also retries automatically with
 * backoff in the background).
 */
export function NetworkErrorPanel({ reason, onRetry }: NetworkErrorPanelProps) {
  const { t } = useLocale();

  return (
    <div className="network-error" role="alert">
      <p className="network-error__message">{t.networkError.message}</p>
      <p className="network-error__reason">{reason}</p>
      <button type="button" className="network-error__retry" onClick={onRetry}>
        {t.networkError.retry}
      </button>
    </div>
  );
}
