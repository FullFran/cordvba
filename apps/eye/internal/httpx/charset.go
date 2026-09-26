package httpx

import (
	"bytes"
	"mime"
	"regexp"
	"strings"
	"unicode/utf8"
)

// xmlDeclCharset matches the encoding attribute of an XML declaration, which
// several Spanish feeds rely on instead of the Content-Type header.
var xmlDeclCharset = regexp.MustCompile(`(?i)<\?xml[^>]*encoding\s*=\s*["']([^"']+)["']`)

// windows1252High maps the 0x80–0x9F range that distinguishes windows-1252 from
// ISO-8859-1. Publishers label feeds as ISO-8859-1 and then emit these bytes
// anyway, so decoding them as windows-1252 is both more forgiving and more
// often correct.
var windows1252High = [32]rune{
	'€', '�', '‚', 'ƒ', '„', '…', '†', '‡',
	'ˆ', '‰', 'Š', '‹', 'Œ', '�', 'Ž', '�',
	'�', '‘', '’', '“', '”', '•', '–', '—',
	'˜', '™', 'š', '›', 'œ', '�', 'ž', 'Ÿ',
}

// DecodeUTF8 returns the response body as valid UTF-8.
//
// It trusts the declared charset, then falls back to sniffing the XML
// declaration, and finally to a validity check. BOE, for one, serves RSS as
// ISO-8859-1; decoding those bytes as UTF-8 turns every accented Spanish word
// into a replacement character.
func (r *Response) DecodeUTF8() string {
	return decode(r.Body, r.ContentType)
}

// decode is the testable core of DecodeUTF8.
func decode(body []byte, contentType string) string {
	switch normalizeCharset(charsetOf(body, contentType)) {
	case "utf-8", "":
		if utf8.Valid(body) {
			return string(body)
		}
		// Declared UTF-8 but not actually UTF-8. Latin-1 is the
		// overwhelmingly likely truth for Spanish sources.
		return fromLatin1(body)
	case "iso-8859-1", "iso-8859-15", "windows-1252", "latin1":
		return fromLatin1(body)
	default:
		// An encoding we do not implement. Return it unchanged when it
		// happens to be valid UTF-8 rather than mangling it.
		if utf8.Valid(body) {
			return string(body)
		}
		return fromLatin1(body)
	}
}

// charsetOf resolves the charset from the Content-Type header, falling back to
// the XML declaration inside the body.
func charsetOf(body []byte, contentType string) string {
	if contentType != "" {
		if _, params, err := mime.ParseMediaType(contentType); err == nil {
			if cs := params["charset"]; cs != "" {
				return cs
			}
		}
	}
	head := body
	if len(head) > 512 {
		head = head[:512]
	}
	if m := xmlDeclCharset.FindSubmatch(head); m != nil {
		return string(bytes.TrimSpace(m[1]))
	}
	return ""
}

// normalizeCharset lowercases and trims a charset label.
func normalizeCharset(cs string) string {
	return strings.ToLower(strings.TrimSpace(strings.Trim(cs, `"'`)))
}

// fromLatin1 decodes ISO-8859-1 / windows-1252 bytes into UTF-8.
func fromLatin1(body []byte) string {
	var sb strings.Builder
	sb.Grow(len(body) * 2)

	for _, b := range body {
		switch {
		case b < 0x80:
			sb.WriteByte(b)
		case b < 0xA0:
			sb.WriteRune(windows1252High[b-0x80])
		default:
			sb.WriteRune(rune(b))
		}
	}
	return sb.String()
}
