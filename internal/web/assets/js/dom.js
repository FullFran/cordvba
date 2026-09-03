// Minimal DOM helpers.
//
// Everything is built with createElement and textContent rather than innerHTML.
// That is not stylistic: source titles and error strings arrive from third
// parties, and a console that renders them as markup is a console that renders
// whatever a publisher decides to put in a title field.

/**
 * Build an element. Attributes are set as properties where the DOM has one and
 * as attributes otherwise; `style` is deliberately not accepted, because an
 * inline style attribute would need 'unsafe-inline' in the policy.
 *
 * @param {string} tag
 * @param {Object} [attrs]
 * @param {...(Node|string|null|undefined)} children
 * @returns {HTMLElement}
 */
export function el(tag, attrs = {}, ...children) {
  const node = document.createElement(tag);

  for (const [key, value] of Object.entries(attrs)) {
    if (value === null || value === undefined || value === false) continue;
    if (key === 'style') throw new Error('inline styles are forbidden; set node.style.* instead');
    if (key === 'class') node.className = value;
    else if (key === 'text') node.textContent = String(value);
    else if (key === 'dataset') Object.assign(node.dataset, value);
    else if (key in node) node[key] = value;
    else node.setAttribute(key, String(value));
  }

  for (const child of children.flat()) {
    if (child === null || child === undefined || child === false) continue;
    node.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
  return node;
}

/** Replace a node's children with the given ones. */
export function fill(node, ...children) {
  node.replaceChildren(...children.flat().filter((c) => c !== null && c !== undefined && c !== false));
  return node;
}

/** Empty a node. */
export function clear(node) {
  node.replaceChildren();
  return node;
}

/** Query one element by id, failing loudly when the markup and the code drift. */
export function byId(id) {
  const node = document.getElementById(id);
  if (!node) throw new Error(`missing element #${id}`);
  return node;
}

export const qs = (selector, root = document) => root.querySelector(selector);
export const qsa = (selector, root = document) => Array.from(root.querySelectorAll(selector));

/** Show or hide via the hidden attribute, which the stylesheet honours. */
export function toggle(node, visible) {
  node.hidden = !visible;
  return node;
}

/** Render a one-line message into a notice box, or hide it when message is falsy. */
export function notice(node, message, kind = 'warn') {
  node.classList.toggle('notice-error', kind === 'error');
  if (!message) {
    clear(node);
    node.hidden = true;
    return node;
  }
  fill(node, typeof message === 'string' ? document.createTextNode(message) : message);
  node.hidden = false;
  return node;
}

/**
 * Debounce a function on the trailing edge. Used for map viewport queries: a
 * pan is dozens of move events and one intended request.
 */
export function debounce(fn, ms) {
  let timer = 0;
  const wrapped = (...args) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), ms);
  };
  wrapped.cancel = () => clearTimeout(timer);
  return wrapped;
}

/** Copy text to the clipboard, falling back to a selectable prompt-free path. */
export async function copyText(text) {
  if (navigator.clipboard && window.isSecureContext) {
    await navigator.clipboard.writeText(text);
    return true;
  }
  const area = el('textarea', { value: text, readOnly: true });
  area.setAttribute('aria-hidden', 'true');
  document.body.append(area);
  area.select();
  const ok = document.execCommand('copy');
  area.remove();
  return ok;
}

/**
 * Hand the browser a file built in this tab. Nothing is uploaded anywhere: the
 * bytes are the ones already on screen.
 */
export function download(filename, mime, text) {
  const url = URL.createObjectURL(new Blob([text], { type: mime }));
  const link = el('a', { href: url, download: filename });
  document.body.append(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
