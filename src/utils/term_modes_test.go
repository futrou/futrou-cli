package utils

import (
	"math/rand"
	"strings"
	"testing"
)

func resetOf(t *TermModes) string { return string(t.Reset()) }

// The reported bug: mouse reporting left on made the new shell print "69;12M35;71;12M...".
func TestTermModesUndoMouseReportingTheRemoteLeftOn(t *testing.T) {
	var m TermModes
	m.Feed([]byte("hello\x1b[?1000;1003;1006h more"))
	got := resetOf(&m)
	for _, want := range []string{"\x1b[?1000l", "\x1b[?1003l", "\x1b[?1006l"} {
		if !strings.Contains(got, want) {
			t.Fatalf("reset %q lacks %q", got, want)
		}
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("attributes are not reset: %q", got)
	}
	if again := resetOf(&m); again != "\x1b[0m" {
		t.Fatalf("a second reset must have nothing left to undo, got %q", again)
	}
}

func TestTermModesLeaveAlternateScreenBracketedPasteAndAHiddenCursor(t *testing.T) {
	var m TermModes
	m.Feed([]byte("\x1b[?1049h\x1b[?2004h\x1b[?25l\x1b[?1h"))
	got := resetOf(&m)
	for _, want := range []string{"\x1b[?1049l", "\x1b[?2004l", "\x1b[?25h", "\x1b[?1l"} {
		if !strings.Contains(got, want) {
			t.Fatalf("reset %q lacks %q", got, want)
		}
	}
}

// A mode the remote switched off again, or never touched, is not touched: a normal exit must not
// move the cursor or leave the screen.
func TestTermModesTouchOnlyWhatIsStillOn(t *testing.T) {
	var m TermModes
	m.Feed([]byte("\x1b[?1049h\x1b[?1049l\x1b[?1000h\x1b[?1000l\x1b[?25l\x1b[?25h"))
	if got := resetOf(&m); got != "\x1b[0m" {
		t.Fatalf("reset = %q, want only the attribute reset", got)
	}
	var none TermModes
	none.Feed([]byte("plain text \x1b[31mred\x1b[0m\r\n"))
	if got := resetOf(&none); got != "\x1b[0m" {
		t.Fatalf("reset = %q", got)
	}
}

// The connection is cut in the middle of a sequence: it is cancelled, and a sequence split
// across two reads is still understood.
func TestTermModesCancelASequenceThatWasCutOff(t *testing.T) {
	var m TermModes
	m.Feed([]byte("text \x1b[?10"))
	if got := resetOf(&m); !strings.HasPrefix(got, "\x18") {
		t.Fatalf("a cut-off sequence is not cancelled: %q", got)
	}
	var split TermModes
	for _, chunk := range []string{"\x1b", "[", "?10", "06", "h"} {
		split.Feed([]byte(chunk))
	}
	if got := resetOf(&split); !strings.Contains(got, "\x1b[?1006l") {
		t.Fatalf("a split sequence was missed: %q", got)
	}
	var osc TermModes
	osc.Feed([]byte("\x1b]0;a title that is cut"))
	if got := resetOf(&osc); !strings.HasPrefix(got, "\x18") {
		t.Fatalf("a cut-off title is not cancelled: %q", got)
	}
}

// Text inside a title or an abandoned sequence never counts as a mode.
func TestTermModesIgnoreModeTextInsideStringsAndAbandonedSequences(t *testing.T) {
	var m TermModes
	m.Feed([]byte("\x1b]0;[?1000h\x07\x1b]2;x\x1b\\\x1b[?1\x1b[?2004h\x1b[?1002\x18 text"))
	got := resetOf(&m)
	if strings.Contains(got, "1000") || strings.Contains(got, "1002") {
		t.Fatalf("a mode that was never switched on is reset: %q", got)
	}
	if !strings.Contains(got, "\x1b[?2004l") {
		t.Fatalf("a real mode was missed: %q", got)
	}
}

func TestTermModesPopTheKeyboardLevelsTheRemotePushed(t *testing.T) {
	var m TermModes
	m.Feed([]byte("\x1b[>1u\x1b[>1u\x1b[<1u"))
	if got := resetOf(&m); !strings.Contains(got, "\x1b[<1u") {
		t.Fatalf("reset = %q, want one level popped", got)
	}
	var none TermModes
	none.Feed([]byte("\x1b[>1u\x1b[<u"))
	if got := resetOf(&none); strings.Contains(got, "u") {
		t.Fatalf("reset = %q, want nothing popped", got)
	}
}

// Whatever bytes arrive, it never panics and always yields a valid reset.
func TestTermModesSurviveAnyInput(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	var m TermModes
	special := []byte("\x1b[?;<>0123456789hlu\x18\x07\\]")
	for i := 0; i < 2000; i++ {
		buf := make([]byte, rng.Intn(200))
		for j := range buf {
			if rng.Intn(3) == 0 {
				buf[j] = special[rng.Intn(len(special))]
			} else {
				buf[j] = byte(rng.Intn(256))
			}
		}
		m.Feed(buf)
		if i%50 == 0 {
			if got := resetOf(&m); !strings.HasSuffix(got, "\x1b[0m") {
				t.Fatalf("invalid reset %q", got)
			}
		}
	}
}
