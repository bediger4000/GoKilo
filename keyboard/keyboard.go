package keyboard

import (
	"os"
)

/* Integer versions of usually single-byte, but
 * sometimes multi-byte, keypresses.
 */
const (
	BACKSPACE   = 127
	ARROW_LEFT  = 1000 + iota
	ARROW_RIGHT = 1000 + iota
	ARROW_UP    = 1000 + iota
	ARROW_DOWN  = 1000 + iota
	DEL_KEY     = 1000 + iota
	HOME_KEY    = 1000 + iota
	END_KEY     = 1000 + iota
	PAGE_UP     = 1000 + iota
	PAGE_DOWN   = 1000 + iota
	CTRL_H      = 'h' & 0x1f
	CTRL_L      = 'l' & 0x1f
	CTRL_F      = 'f' & 0x1f
	CTRL_Q      = 'q' & 0x1f
	CTRL_S      = 's' & 0x1f
	ESCAPE      = '\x1b'
)

// ReadKey reads a possibly multi-byte keypress from stdin, returning an
// int (the const values above) that represents the keypress.
func ReadKey() (int, error) {
	var buffer [1]byte
	var cc int
	var err error
	for cc, err = os.Stdin.Read(buffer[:]); cc != 1; cc, err = os.Stdin.Read(buffer[:]) {
	}
	if err != nil {
		return -1, err
	}
	if buffer[0] == ESCAPE {
		return readEscapeSequence()
	}
	return int(buffer[0]), nil
}

func arrowKeyDecode(k byte) (int, error) {
	switch k {
	case 'A':
		return ARROW_UP, nil
	case 'B':
		return ARROW_DOWN, nil
	case 'C':
		return ARROW_RIGHT, nil
	case 'D':
		return ARROW_LEFT, nil
	case 'H':
		return HOME_KEY, nil
	case 'F':
		return END_KEY, nil
	}
	return int(k), nil
}

func tildeDecode(b byte) (int, error) {
	switch b {
	case '1':
		return HOME_KEY, nil
	case '3':
		return DEL_KEY, nil
	case '4':
		return END_KEY, nil
	case '5':
		return PAGE_UP, nil
	case '6':
		return PAGE_DOWN, nil
	case '7':
		return HOME_KEY, nil
	case '8':
		return END_KEY, nil
	}
	return int(b), nil
}

func readEscapeSequence() (int, error) {
	var seq [2]byte
	var buffer [1]byte
	var cc int

	if cc, _ = os.Stdin.Read(seq[:]); cc != 2 {
		return ESCAPE, nil
	}

	if seq[0] == '[' {
		if seq[1] >= '0' && seq[1] <= '9' {
			if cc, _ = os.Stdin.Read(buffer[:]); cc != 1 {
				return '\x1b', nil
			}
			if buffer[0] == '~' {
				return tildeDecode(seq[1])
			}
			// XXX - falls all the way through
		} else {
			return arrowKeyDecode(seq[1])
		}
		// XXX - falls all the way through
	} else if seq[0] == '0' {
		switch seq[1] {
		case 'H':
			return HOME_KEY, nil
		case 'F':
			return END_KEY, nil
		}
		// XXX - is it correct to fall through?
	}

	return ESCAPE, nil
}
