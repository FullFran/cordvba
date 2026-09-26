package tui_test

import (
	"bytes"
	"testing"

	"github.com/FullFran/cordvba/apps/eye/internal/tui"
)

// The decoder is the whole of eye's input handling: there is no terminal
// library behind it (ADR-0004), so every sequence a keyboard can produce has to
// be recognised here or the key does nothing.
func TestDecodeKeys(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want []tui.Key
	}{
		{
			name: "printable characters",
			in:   "abc",
			want: []tui.Key{
				{Code: tui.KeyRune, Rune: 'a'},
				{Code: tui.KeyRune, Rune: 'b'},
				{Code: tui.KeyRune, Rune: 'c'},
			},
		},
		{
			name: "a multi-byte rune arrives whole",
			in:   "ó",
			want: []tui.Key{{Code: tui.KeyRune, Rune: 'ó'}},
		},
		{
			name: "ctrl-c",
			in:   "\x03",
			want: []tui.Key{{Code: tui.KeyCtrlC}},
		},
		{
			name: "enter, both spellings",
			in:   "\r\n",
			want: []tui.Key{{Code: tui.KeyEnter}, {Code: tui.KeyEnter}},
		},
		{
			name: "tab",
			in:   "\t",
			want: []tui.Key{{Code: tui.KeyTab}},
		},
		{
			name: "back tab",
			in:   "\x1b[Z",
			want: []tui.Key{{Code: tui.KeyBackTab}},
		},
		{
			name: "backspace, both spellings",
			in:   "\x7f\x08",
			want: []tui.Key{{Code: tui.KeyBackspace}, {Code: tui.KeyBackspace}},
		},
		{
			name: "arrows as CSI",
			in:   "\x1b[A\x1b[B\x1b[C\x1b[D",
			want: []tui.Key{
				{Code: tui.KeyUp},
				{Code: tui.KeyDown},
				{Code: tui.KeyRight},
				{Code: tui.KeyLeft},
			},
		},
		{
			name: "arrows as SS3, which is what an application-mode keypad sends",
			in:   "\x1bOA\x1bOB",
			want: []tui.Key{{Code: tui.KeyUp}, {Code: tui.KeyDown}},
		},
		{
			name: "home and end as CSI letters",
			in:   "\x1b[H\x1b[F",
			want: []tui.Key{{Code: tui.KeyHome}, {Code: tui.KeyEnd}},
		},
		{
			name: "home, end, page up, page down and delete as tilde sequences",
			in:   "\x1b[1~\x1b[4~\x1b[5~\x1b[6~\x1b[3~",
			want: []tui.Key{
				{Code: tui.KeyHome},
				{Code: tui.KeyEnd},
				{Code: tui.KeyPageUp},
				{Code: tui.KeyPageDown},
				{Code: tui.KeyDelete},
			},
		},
		{
			name: "modified arrows still move",
			in:   "\x1b[1;5A",
			want: []tui.Key{{Code: tui.KeyUp}},
		},
		{
			name: "a lone escape is the escape key",
			in:   "\x1b",
			want: []tui.Key{{Code: tui.KeyEscape}},
		},
		{
			name: "escape followed by a key is still escape then the key",
			in:   "\x1ba",
			want: []tui.Key{{Code: tui.KeyEscape}, {Code: tui.KeyRune, Rune: 'a'}},
		},
		{
			name: "a sequence this build does not know is dropped, not typed",
			in:   "\x1b[200~",
			want: nil,
		},
		{
			name: "a key after an unknown sequence still arrives",
			in:   "\x1b[200~x",
			want: []tui.Key{{Code: tui.KeyRune, Rune: 'x'}},
		},
		{
			name: "nothing",
			in:   "",
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, rest := tui.DecodeKeys([]byte(tc.in))
			if len(rest) != 0 {
				t.Errorf("DecodeKeys(%q) held back %q", tc.in, rest)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("DecodeKeys(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("key %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// A read can split an escape sequence down the middle. The half that arrived
// has to be held rather than delivered as an Escape and a stray bracket, which
// on the map would pan and then type a filter.
func TestDecodeKeysHoldsAPartialSequence(t *testing.T) {
	t.Parallel()

	cases := []string{"\x1b[", "\x1b[1", "\x1b[1;5", "\x1bO"}

	for _, in := range cases {
		got, rest := tui.DecodeKeys([]byte(in))
		if len(got) != 0 {
			t.Errorf("DecodeKeys(%q) produced %+v before the sequence was complete", in, got)
		}
		if !bytes.Equal(rest, []byte(in)) {
			t.Errorf("DecodeKeys(%q) held %q, want the whole partial sequence", in, rest)
		}
	}
}

func TestDecodeKeysCompletesAHeldSequence(t *testing.T) {
	t.Parallel()

	_, rest := tui.DecodeKeys([]byte("x\x1b["))
	got, rest := tui.DecodeKeys(append(rest, 'A'))

	if len(rest) != 0 {
		t.Errorf("the completed sequence left %q behind", rest)
	}
	if len(got) != 1 || got[0].Code != tui.KeyUp {
		t.Errorf("the rejoined sequence decoded to %+v, want Up", got)
	}
}

// An incomplete multi-byte rune must be held too, or the terminal's own
// encoding turns into two pieces of rubbish.
func TestDecodeKeysHoldsAPartialRune(t *testing.T) {
	t.Parallel()

	whole := []byte("ó")
	got, rest := tui.DecodeKeys(whole[:1])

	if len(got) != 0 {
		t.Errorf("half a rune decoded to %+v", got)
	}
	if !bytes.Equal(rest, whole[:1]) {
		t.Errorf("held %q, want the first byte back", rest)
	}
}
