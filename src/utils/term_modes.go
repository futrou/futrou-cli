package utils

import (
	"strconv"
	"strings"
)

// TermModes follows the terminal modes a remote program switches on in the local terminal
// (mouse reporting, bracketed paste, the alternate screen, a hidden cursor, ...) by reading
// the output that goes to the screen. When the connection ends or is replaced by a new shell,
// Reset undoes exactly those, so the new shell does not get mouse reports and escape
// sequences typed into it, and a sequence cut in half by the drop is cancelled instead of
// swallowing what follows. Nothing is changed for a mode the remote never touched, so a
// normal exit leaves the screen as it is.
type TermModes struct {
	state      int // 0 text, 1 after ESC, 2 inside CSI, 3 inside a string (OSC, DCS, ...), 4 ESC inside a string
	marker     byte
	params     []byte
	modes      map[int]bool // private modes (CSI ? n h/l) the remote set or reset
	kittyDepth int          // kitty keyboard protocol levels pushed and not popped
}

const maxCSIParams = 64

// Feed reads output that is being written to the terminal. Sequences may be split across calls.
func (t *TermModes) Feed(p []byte) {
	for _, b := range p {
		t.feedByte(b)
	}
}

func (t *TermModes) feedByte(b byte) {
	switch t.state {
	case 0:
		if b == 0x1b {
			t.state = 1
		}
	case 1:
		switch b {
		case '[':
			t.state, t.marker, t.params = 2, 0, t.params[:0]
		case ']', 'P', 'X', '^', '_':
			t.state = 3
		case 0x1b:
		default:
			t.state = 0
		}
	case 2:
		switch {
		case b >= 0x30 && b <= 0x3f:
			if len(t.params) == 0 && t.marker == 0 && (b == '?' || b == '>' || b == '<' || b == '=') {
				t.marker = b
			} else if len(t.params) < maxCSIParams {
				t.params = append(t.params, b)
			}
		case b >= 0x20 && b <= 0x2f:
			// intermediate bytes: part of the sequence, nothing to record
		case b >= 0x40 && b <= 0x7e:
			t.state = 0
			t.csi(b)
		case b == 0x1b:
			t.state = 1 // the sequence was abandoned for a new one
		case b == 0x18 || b == 0x1a:
			t.state = 0 // CAN and SUB cancel a sequence
		}
	case 3:
		switch b {
		case 0x07, 0x18, 0x1a:
			t.state = 0
		case 0x1b:
			t.state = 4
		}
	case 4:
		if b == '\\' {
			t.state = 0
		} else {
			t.state = 1
			t.feedByte(b)
		}
	}
}

func (t *TermModes) csi(final byte) {
	switch {
	case t.marker == '?' && (final == 'h' || final == 'l'):
		if t.modes == nil {
			t.modes = map[int]bool{}
		}
		for _, field := range strings.Split(string(t.params), ";") {
			if n, err := strconv.Atoi(field); err == nil {
				t.modes[n] = final == 'h'
			}
		}
	case t.marker == '>' && final == 'u':
		t.kittyDepth++
	case t.marker == '<' && final == 'u':
		n := 1
		if v, err := strconv.Atoi(string(t.params)); err == nil && v > 0 {
			n = v
		}
		t.kittyDepth = max(0, t.kittyDepth-n)
	}
}

// resetModes are the modes whose being left on breaks a new shell; only those the remote set are reset.
var resetModes = []int{1000, 1001, 1002, 1003, 1004, 1005, 1006, 1015, 1016, 2004, 2026, 1, 1049, 1047, 47}

// Reset returns what to write to the terminal to undo what the remote left behind, and forgets it.
func (t *TermModes) Reset() []byte {
	var out strings.Builder
	if t.state != 0 {
		out.WriteByte(0x18) // cancels a sequence that was cut off
		t.state = 0
	}
	for _, n := range resetModes {
		if t.modes[n] {
			out.WriteString("\x1b[?" + strconv.Itoa(n) + "l")
		}
	}
	if on, hidden := t.modes[25]; hidden && !on { // the cursor was hidden
		out.WriteString("\x1b[?25h")
	}
	if t.kittyDepth > 0 {
		out.WriteString("\x1b[<" + strconv.Itoa(t.kittyDepth) + "u")
	}
	out.WriteString("\x1b[0m") // colors and attributes the remote left set
	t.modes, t.kittyDepth = nil, 0
	return []byte(out.String())
}
