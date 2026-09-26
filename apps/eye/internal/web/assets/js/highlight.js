// A hand-written JSON highlighter.
//
// A syntax highlighting library would be the fourth dependency of a project
// whose whole point is that it does not have a fourth dependency. This is
// forty lines and produces text nodes and spans, never markup, so a hostile
// string in a payload is a hostile string on screen and nothing more.

import { el } from './dom.js';

// One pass over the pretty-printed text. The alternatives are ordered so that
// a string is consumed whole before its contents can look like anything else,
// and the optional colon that follows decides key from value.
const TOKEN = /("(?:\\.|[^"\\])*")(\s*:)?|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g;

/**
 * Render a value as a highlighted <pre>.
 *
 * @param {unknown} value any JSON-serialisable value
 * @param {number} [indent]
 * @returns {HTMLPreElement}
 */
export function jsonBlock(value, indent = 2) {
  const pre = el('pre', { class: 'json' });
  let text;
  try {
    text = JSON.stringify(value, null, indent);
  } catch (err) {
    pre.textContent = `unserialisable value: ${err.message}`;
    return pre;
  }
  if (text === undefined) text = String(value);

  let cursor = 0;
  TOKEN.lastIndex = 0;
  for (let m = TOKEN.exec(text); m !== null; m = TOKEN.exec(text)) {
    if (m.index > cursor) pre.append(punctuation(text.slice(cursor, m.index)));

    const [, string, colon, literal, number] = m;
    if (string !== undefined) {
      pre.append(el('span', { class: colon ? 'j-key' : 'j-str', text: string }));
      if (colon) pre.append(punctuation(colon));
    } else if (literal !== undefined) {
      pre.append(el('span', { class: 'j-lit', text: literal }));
    } else {
      pre.append(el('span', { class: 'j-num', text: number }));
    }
    cursor = m.index + m[0].length;
  }
  if (cursor < text.length) pre.append(punctuation(text.slice(cursor)));

  return pre;
}

function punctuation(text) {
  return el('span', { class: 'j-pun', text });
}
