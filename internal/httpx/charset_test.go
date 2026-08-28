package httpx

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDecodeLatin1(t *testing.T) {
	t.Parallel()

	// "Córdoba, Diputación" as ISO-8859-1 bytes.
	latin1 := []byte{
		'C', 0xF3, 'r', 'd', 'o', 'b', 'a', ',', ' ',
		'D', 'i', 'p', 'u', 't', 'a', 'c', 'i', 0xF3, 'n',
	}

	cases := []struct {
		name        string
		body        []byte
		contentType string
		want        string
	}{
		{
			name:        "charset from header",
			body:        latin1,
			contentType: "text/xml; charset=ISO-8859-1",
			want:        "Córdoba, Diputación",
		},
		{
			name:        "charset from xml declaration",
			body:        append([]byte(`<?xml version="1.0" encoding="ISO-8859-1"?>`), latin1...),
			contentType: "text/xml",
			want:        `<?xml version="1.0" encoding="ISO-8859-1"?>` + "Córdoba, Diputación",
		},
		{
			name:        "windows-1252 label",
			body:        latin1,
			contentType: "application/rss+xml; charset=windows-1252",
			want:        "Córdoba, Diputación",
		},
		{
			name:        "utf-8 passes through untouched",
			body:        []byte("Córdoba, Diputación"),
			contentType: "application/json; charset=utf-8",
			want:        "Córdoba, Diputación",
		},
		{
			name:        "no charset and valid utf-8",
			body:        []byte("Córdoba"),
			contentType: "",
			want:        "Córdoba",
		},
		{
			name:        "declared utf-8 but actually latin-1",
			body:        latin1,
			contentType: "text/xml; charset=utf-8",
			want:        "Córdoba, Diputación",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := decode(tc.body, tc.contentType)
			if got != tc.want {
				t.Errorf("decode() = %q, want %q", got, tc.want)
			}
			if !utf8.ValidString(got) {
				t.Error("decode() produced invalid UTF-8")
			}
		})
	}
}

func TestDecodeWindows1252SmartQuotes(t *testing.T) {
	t.Parallel()

	// 0x93 and 0x94 are smart quotes in windows-1252 and undefined in
	// ISO-8859-1. Publishers emit them under either label.
	body := []byte{0x93, 'V', 'e', 'l', 0xE1, 0x94}

	got := decode(body, "text/xml; charset=ISO-8859-1")
	if want := "“Velá”"; got != want {
		t.Errorf("decode() = %q, want %q", got, want)
	}
}

func TestDecodeUTF8UsesResponseMetadata(t *testing.T) {
	t.Parallel()

	r := &Response{Body: []byte{'V', 'e', 'l', 0xE1}, ContentType: "text/xml; charset=ISO-8859-1"}
	if got := r.DecodeUTF8(); !strings.Contains(got, "Velá") {
		t.Errorf("DecodeUTF8() = %q, want it to contain %q", got, "Velá")
	}
}
