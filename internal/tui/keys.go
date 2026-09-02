package tui

import (
	"unicode/utf8"
)

// KeyCode names a key that is not a printable character.
type KeyCode int

// The keys the cockpit understands. Anything else a terminal can send is
// dropped rather than delivered as rubbish.
const (
	// KeyRune is a printable character; the rune is in Key.Rune.
	KeyRune KeyCode = iota
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyEnter
	KeyTab
	KeyBackTab
	KeyBackspace
	KeyDelete
	KeyEscape
	KeyCtrlC
)

// Key is one keystroke.
type Key struct {
	Code KeyCode
	Rune rune
}

// Control bytes a terminal sends outside any escape sequence.
const (
	byteCtrlC     = 0x03
	byteBackspace = 0x08
	byteTab       = 0x09
	byteNewline   = 0x0a
	byteReturn    = 0x0d
	byteEscape    = 0x1b
	byteDelete    = 0x7f
)

// DecodeKeys turns a buffer of terminal input into keystrokes.
//
// It is a pure function over bytes so it can be tested without a terminal,
// which matters more here than usual: eye decodes escape sequences itself
// rather than spending a dependency on a terminal library (ADR-0004), so this
// is the only place a wrong byte order can be caught.
//
// rest is the tail that could not yet be decoded — a split escape sequence or
// half a UTF-8 rune. The caller must prepend it to the next read. Holding it is
// the difference between a slow terminal sending a Left arrow and eye reading
// an Escape followed by somebody typing "[D".
func DecodeKeys(b []byte) (keys []Key, rest []byte) {
	for i := 0; i < len(b); {
		// An escape sequence: either complete, or the tail to hold.
		if b[i] == byteEscape {
			key, n, ok, partial := decodeEscape(b[i:])
			if partial {
				return keys, b[i:]
			}
			if ok {
				keys = append(keys, key)
			}
			i += n
			continue
		}

		if key, ok := decodeControl(b[i]); ok {
			keys = append(keys, key)
			i++
			continue
		}

		r, n := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && n <= 1 {
			// Either an invalid byte or the start of a rune whose
			// remaining bytes have not arrived. Holding a possibly
			// complete-but-invalid byte would stall forever, so only a
			// plausible prefix is held back.
			if utf8.RuneStart(b[i]) && len(b)-i < utf8.UTFMax {
				return keys, b[i:]
			}
			i++
			continue
		}

		keys = append(keys, Key{Code: KeyRune, Rune: r})
		i += n
	}
	return keys, nil
}

// decodeControl recognises the single-byte control keys.
func decodeControl(c byte) (Key, bool) {
	switch c {
	case byteCtrlC:
		return Key{Code: KeyCtrlC}, true
	case byteReturn, byteNewline:
		return Key{Code: KeyEnter}, true
	case byteTab:
		return Key{Code: KeyTab}, true
	case byteBackspace, byteDelete:
		return Key{Code: KeyBackspace}, true
	default:
		return Key{}, false
	}
}

// decodeEscape reads one escape sequence from the front of b.
//
// n is how many bytes it consumed, ok reports whether it produced a key, and
// partial reports that the sequence is incomplete and the caller should wait
// for more input rather than guess.
func decodeEscape(b []byte) (key Key, n int, ok, partial bool) {
	// A lone ESC at the end of the buffer is the Escape key. Terminals send
	// a sequence in a single write, so a trailing ESC is a keystroke rather
	// than the start of something.
	if len(b) == 1 {
		return Key{Code: KeyEscape}, 1, true, false
	}

	switch b[1] {
	case '[':
		return decodeCSI(b)
	case 'O':
		// SS3: what an application-mode keypad sends for the arrows.
		if len(b) < 3 {
			return Key{}, 0, false, true
		}
		if code, found := csiFinal[b[2]]; found {
			return Key{Code: code}, 3, true, false
		}
		return Key{}, 3, false, false
	default:
		// ESC followed by anything else: Escape, then that key on the
		// next pass.
		return Key{Code: KeyEscape}, 1, true, false
	}
}

// csiFinal maps the letter that ends a cursor sequence to its key.
var csiFinal = map[byte]KeyCode{
	'A': KeyUp,
	'B': KeyDown,
	'C': KeyRight,
	'D': KeyLeft,
	'H': KeyHome,
	'F': KeyEnd,
	'Z': KeyBackTab,
}

// csiTilde maps the numeric parameter of a "~" sequence to its key.
var csiTilde = map[string]KeyCode{
	"1": KeyHome,
	"7": KeyHome,
	"3": KeyDelete,
	"4": KeyEnd,
	"8": KeyEnd,
	"5": KeyPageUp,
	"6": KeyPageDown,
}

// decodeCSI reads an ESC [ … sequence.
func decodeCSI(b []byte) (key Key, n int, ok, partial bool) {
	// Parameters are digits and semicolons; the sequence ends on the first
	// byte in 0x40..0x7e. Modifiers ("\x1b[1;5A" for ctrl-up) are parsed and
	// then ignored: the cockpit has no modified bindings, and a ctrl-arrow
	// should still pan rather than do nothing.
	for i := 2; i < len(b); i++ {
		c := b[i]
		if (c >= '0' && c <= '9') || c == ';' {
			continue
		}
		if c < 0x40 || c > 0x7e {
			return Key{}, i + 1, false, false
		}

		params := string(b[2:i])
		if c == '~' {
			if code, found := csiTilde[firstParam(params)]; found {
				return Key{Code: code}, i + 1, true, false
			}
			return Key{}, i + 1, false, false
		}
		if code, found := csiFinal[c]; found {
			return Key{Code: code}, i + 1, true, false
		}
		return Key{}, i + 1, false, false
	}
	return Key{}, 0, false, true
}

// firstParam returns the parameter before the first semicolon.
func firstParam(params string) string {
	for i := range len(params) {
		if params[i] == ';' {
			return params[:i]
		}
	}
	return params
}
