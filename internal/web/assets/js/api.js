// The API client.
//
// The console never talks to a backend of its own: it calls the same /v1 that
// any curl one-liner calls, on the same origin. So there is no server-side
// session, no proxy, and nothing the page can see that a shell could not.
//
// The token lives in localStorage and travels in an Authorization header. It is
// never placed in a URL, because URLs end up in browser history, in referrers
// and in server logs, and a token in a log is a token that has leaked.

const TOKEN_KEY = 'eye.token';

/** An HTTP failure carrying whatever the API said about it. */
export class ApiError extends Error {
  constructor(status, body, path) {
    const detail = body && (body.detail || body.error);
    super(detail ? `${status}: ${detail}` : `${status}`);
    this.name = 'ApiError';
    this.status = status;
    this.body = body || {};
    this.path = path;
  }

  /** The short machine-ish error the API puts in `error`. */
  get code() {
    return this.body.error || '';
  }

  /** The long human explanation, when the API bothered to write one. */
  get detail() {
    return this.body.detail || '';
  }
}

const listeners = new Set();

/** Register a callback fired the first time a call is refused for auth. */
export function onUnauthorized(fn) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

export function getToken() {
  try {
    return window.localStorage.getItem(TOKEN_KEY) || '';
  } catch {
    // Private-mode browsers throw rather than return null. A console with no
    // storage still works; it just asks again on every load.
    return '';
  }
}

export function setToken(token) {
  try {
    if (token) window.localStorage.setItem(TOKEN_KEY, token);
    else window.localStorage.removeItem(TOKEN_KEY);
  } catch {
    /* nothing to do: the header below still carries the in-memory value */
  }
  memoryToken = token || '';
}

export function clearToken() {
  setToken('');
}

let memoryToken = '';

function token() {
  return getToken() || memoryToken;
}

/** Absolute URL for a path plus query, with empty parameters dropped. */
export function urlFor(path, params = {}) {
  const url = new URL(path, window.location.origin);
  for (const [key, value] of Object.entries(params)) {
    if (value === null || value === undefined || value === '') continue;
    url.searchParams.set(key, String(value));
  }
  return url;
}

/**
 * The exact request this console would make, as a shell command.
 *
 * The token is referenced as an environment variable rather than pasted in:
 * the point of the button is a reproducible request, and a copied secret in a
 * chat window is not part of that.
 */
export function curlFor(path, params = {}) {
  const url = urlFor(path, params);
  const parts = ['curl -sS', `'${url.toString().replace(/'/g, "'\\''")}'`];
  if (token()) parts.push(`\\\n  -H "Authorization: Bearer $EYE_API_TOKEN"`);
  return parts.join(' ');
}

/**
 * GET a JSON endpoint.
 *
 * @param {string} path e.g. "/v1/records"
 * @param {Object} [params]
 * @param {{signal?: AbortSignal}} [options]
 */
export async function get(path, params = {}, options = {}) {
  const headers = { Accept: 'application/json' };
  const bearer = token();
  if (bearer) headers.Authorization = `Bearer ${bearer}`;

  let response;
  try {
    response = await fetch(urlFor(path, params), {
      method: 'GET',
      headers,
      cache: 'no-store',
      credentials: 'omit',
      redirect: 'error',
      signal: options.signal,
    });
  } catch (err) {
    if (err && err.name === 'AbortError') throw err;
    throw new ApiError(0, { error: 'unreachable', detail: String(err && err.message ? err.message : err) }, path);
  }

  const body = await parse(response);

  if (response.status === 401) {
    for (const fn of listeners) fn();
    throw new ApiError(401, body, path);
  }
  if (!response.ok) throw new ApiError(response.status, body, path);

  return body;
}

async function parse(response) {
  const text = await response.text();
  if (!text) return {};
  try {
    return JSON.parse(text);
  } catch {
    return { error: 'unparseable response', detail: text.slice(0, 400) };
  }
}

/**
 * Pull the array out of a response whatever the envelope calls it.
 *
 * The contract names a key per endpoint; accepting a bare array too means a
 * shape change costs a wrong-looking table rather than a blank page.
 */
export function listFrom(payload, ...keys) {
  if (Array.isArray(payload)) return payload;
  if (!payload || typeof payload !== 'object') return [];
  for (const key of [...keys, 'items', 'results', 'data']) {
    if (Array.isArray(payload[key])) return payload[key];
  }
  return [];
}

/** First present field, so a renamed field degrades to a blank and not a crash. */
export function pick(object, ...names) {
  if (!object || typeof object !== 'object') return undefined;
  for (const name of names) {
    const value = object[name];
    if (value !== undefined && value !== null && value !== '') return value;
  }
  return undefined;
}
