package editor

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"time"
	"unicode"

	"GoKilo/filemgt"
	"GoKilo/highlighter"
	"GoKilo/keyboard"
	"GoKilo/row"
	"GoKilo/screen"
)

/*** defines ***/

const kiloVersion = "0.0.2"
const kiloQuitTimes = 3

// Editor instances keep track of several things right now,
// from cursor coords inside of file to screen size to
// file name read from or to save to. Probably needs refactoring.
type Editor struct {
	cx            int
	cy            int
	rx            int
	rowoff        int
	coloff        int
	screenRows    int
	screenCols    int
	numRows       int
	rows          []*row.Row
	Dirty         bool
	Filename      string
	statusmsg     string
	statusMsgTime time.Time
	syntax        *highlighter.Syntax
}

// UpdateAllSyntax redoes all the syntax highlighting, for
// every line in the edited file.
func (e *Editor) UpdateAllSyntax() {
	e.syntax = highlighter.SelectSyntaxHighlight(e.Filename)
	inComment := false
	for _, row := range e.rows {
		e.syntax.UpdateSyntax(row, inComment)
		inComment = row.HlOpenComment
	}
}

func (e *Editor) updateSyntax(at int) {
	inComment := at > 0 && e.rows[at-1].HlOpenComment
	for at < e.numRows && e.syntax.UpdateSyntax(e.rows[at], inComment) {
		at++
		inComment = at > 0 && e.rows[at-1].HlOpenComment
	}
}

// AppendRow puts a line of text from the edited file
// at the end of the internal representation of the file.
func (e *Editor) AppendRow(s []byte) {
	e.insertRow(e.numRows, s)
}

func (e *Editor) insertRow(at int, s []byte) {
	if at < 0 || at > e.numRows {
		return
	}
	var r row.Row
	r.Chars = s
	r.Size = len(s)

	switch at {
	case 0:
		t := make([]*row.Row, 1)
		t[0] = &r
		e.rows = append(t, e.rows...)
	case e.numRows:
		e.rows = append(e.rows, &r)
	default:
		t := make([]*row.Row, 1)
		t[0] = &r
		e.rows = append(e.rows[:at], append(t, e.rows[at:]...)...)
	}

	e.rows[at].UpdateRow()
	e.updateSyntax(at)
	e.numRows++
	e.Dirty = true
}

func (e *Editor) delRow(at int) {
	if at < 0 || at > e.numRows {
		return
	}
	e.rows = append(e.rows[:at], e.rows[at+1:]...)
	e.numRows--
	e.Dirty = true
}

func (e *Editor) insertChar(c byte) {
	if e.cy == e.numRows {
		var emptyRow []byte
		e.AppendRow(emptyRow)
	}
	e.rows[e.cy].RowInsertChar(e.cx, c)
	e.updateSyntax(e.cy)
	e.Dirty = true
	e.cx++
}

func (e *Editor) insertNewLine() {
	if e.cx == 0 {
		e.insertRow(e.cy, make([]byte, 0))
	} else {
		e.insertRow(e.cy+1, e.rows[e.cy].Chars[e.cx:])
		e.rows[e.cy].Chars = e.rows[e.cy].Chars[:e.cx]
		e.rows[e.cy].Size = len(e.rows[e.cy].Chars)
		e.rows[e.cy].UpdateRow()
		e.updateSyntax(e.cy)
	}
	e.cy++
	e.cx = 0
}

func (e *Editor) delChar() {
	if e.cy == e.numRows {
		return
	}
	if e.cx == 0 && e.cy == 0 {
		return
	}
	if e.cx > 0 {
		e.rows[e.cy].RowDelChar(e.cx - 1)
		e.updateSyntax(e.cy)
		e.cx--
	} else {
		e.cx = e.rows[e.cy-1].Size
		e.rows[e.cy-1].RowAppendString(e.rows[e.cy].Chars)
		e.updateSyntax(e.cy - 1)
		e.Dirty = true
		e.delRow(e.cy)
		e.cy--
	}
	e.Dirty = true
}

func (e *Editor) rowsToString() (buf string, totlen int) {
	for _, aRow := range e.rows {
		totlen += aRow.Size + 1
		buf += string(aRow.Chars) + "\n"
	}
	return
}

/*** find ***/

// XXX - should these all be Editor instance variables?
var lastMatch = -1
var direction = 1
var savedHlLine int
var savedHl []byte

func (e *Editor) findCallback(qry []byte, key int) {

	if savedHlLine > 0 {
		copy(e.rows[savedHlLine].Hl, savedHl)
		savedHlLine = 0
		savedHl = nil
	}

	switch key {
	case '\r', keyboard.ESCAPE:
		lastMatch = -1
		direction = 1
		return
	case keyboard.ARROW_RIGHT, keyboard.ARROW_DOWN:
		direction = 1
	case keyboard.ARROW_LEFT, keyboard.ARROW_UP:
		direction = -1
	default:
		lastMatch = -1
		direction = 1
	}

	if lastMatch == -1 {
		direction = 1
	}
	current := lastMatch

	for range e.rows {
		current += direction
		if current == -1 {
			current = e.numRows - 1
		} else if current == e.numRows {
			current = 0
		}
		thisRow := e.rows[current]
		x := bytes.Index(thisRow.Render, qry)
		if x > -1 {
			lastMatch = current
			e.cy = current
			e.cx = thisRow.RowRxToCx(x)
			e.rowoff = e.numRows
			savedHlLine = current
			savedHl = make([]byte, thisRow.Rsize)
			copy(savedHl, thisRow.Hl)
			max := x + len(qry)
			for i := x; i < max; i++ {
				thisRow.Hl[i] = highlighter.HL_MATCH
			}
			break
		}
	}
}

func find(e *Editor) {
	savedCx := e.cx
	savedCy := e.cy
	savedColoff := e.coloff
	savedRowoff := e.rowoff
	query, _ := e.prompt("Search: %s (ESC/Arrows/Enter)", e.findCallback)
	// XXX - what to do with the error return here?
	if query == "" {
		e.cx = savedCx
		e.cy = savedCy
		e.coloff = savedColoff
		e.rowoff = savedRowoff
	}
}

/*** input ***/

func (e *Editor) prompt(prompt string, callback func([]byte, int)) (string, error) {
	var buf []byte

	for {
		e.SetStatusMessage(prompt, buf)
		e.RefreshScreen()

		c, err := keyboard.ReadKey()
		if err != nil {
			return "", err
		}

		switch c {
		case keyboard.DEL_KEY, keyboard.CTRL_H, keyboard.BACKSPACE:
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
			}
		case keyboard.ESCAPE:
			e.SetStatusMessage("")
			if callback != nil {
				callback(buf, c)
			}
			return "", nil
		case '\r':
			if len(buf) != 0 {
				e.SetStatusMessage("")
				if callback != nil {
					callback(buf, c)
				}
				return string(buf), nil
			}
		default:
			if unicode.IsPrint(rune(c)) {
				buf = append(buf, byte(c))
			}
		}
		if callback != nil {
			callback(buf, c)
		}
	}
}

func (e *Editor) moveCursor(key int) {
	switch key {
	case keyboard.ARROW_LEFT:
		if e.cx != 0 {
			e.cx--
		} else if e.cy > 0 {
			e.cy--
			e.cx = e.rows[e.cy].Size
		}
	case keyboard.ARROW_RIGHT:
		if e.cy < e.numRows {
			if e.cx < e.rows[e.cy].Size {
				e.cx++
			} else if e.cx == e.rows[e.cy].Size {
				e.cy++
				e.cx = 0
			}
		}
	case keyboard.ARROW_UP:
		if e.cy != 0 {
			e.cy--
		}
	case keyboard.ARROW_DOWN:
		if e.cy < e.numRows {
			e.cy++
		}
	}

	rowlen := 0
	if e.cy < e.numRows {
		rowlen = e.rows[e.cy].Size
	}
	if e.cx > rowlen {
		e.cx = rowlen
	}
}

var quitTimes = kiloQuitTimes

// ProcessKeypress gets a (possibly multi-byte) keypress from keyboard, then
// decides what to do to Editor's internal state based on that byte or bytes.
func (e *Editor) ProcessKeypress() (bool, error) {
	c, err := keyboard.ReadKey()
	if err != nil {
		return false, err
	}
	switch c {
	case '\r':
		e.insertNewLine()
	case keyboard.CTRL_Q:
		return e.processQuit()
	case keyboard.CTRL_S:
		return e.saveFile()
	case keyboard.HOME_KEY:
		e.cx = 0
	case keyboard.END_KEY:
		if e.cy < e.numRows {
			e.cx = e.rows[e.cy].Size
		}
	case keyboard.CTRL_F:
		find(e)
	case keyboard.CTRL_H, keyboard.BACKSPACE, keyboard.DEL_KEY:
		e.deleteSomething(c)
	case keyboard.PAGE_UP, keyboard.PAGE_DOWN:
		e.moveScreenful(c)
	case keyboard.ARROW_UP, keyboard.ARROW_DOWN,
		keyboard.ARROW_LEFT, keyboard.ARROW_RIGHT:
		e.moveCursor(c)
	case keyboard.CTRL_L:
	case keyboard.ESCAPE:
	default:
		e.insertChar(byte(c))
	}
	quitTimes = kiloQuitTimes
	return true, nil
}

func (e *Editor) deleteSomething(c int) {
	if c == keyboard.DEL_KEY {
		e.moveCursor(keyboard.ARROW_RIGHT)
	}
	e.delChar()
}

func (e *Editor) processQuit() (bool, error) {
	if e.Dirty && quitTimes > 0 {
		e.SetStatusMessage("Warning!!! File has unsaved changes. Press Ctrl-Q %d more times to quit.", quitTimes)
		quitTimes--
		return true, nil
	}
	return false, nil
}

func (e *Editor) moveScreenful(c int) {
	dir := keyboard.ARROW_DOWN
	if c == keyboard.PAGE_UP {
		e.cy = e.rowoff
		dir = keyboard.ARROW_UP
	} else {
		e.cy = e.rowoff + e.screenRows - 1
		if e.cy > e.numRows {
			e.cy = e.numRows
		}
	}
	for times := e.screenRows; times > 0; times-- {
		e.moveCursor(dir)
	}
}

func (e *Editor) saveFile() (bool, error) {
	if e.Filename == "" {
		var err error
		e.Filename, err = e.prompt("Save as: %q", nil)
		if e.Filename == "" {
			e.SetStatusMessage("Save aborted")
			return true, err
		}
		if err != nil {
			e.SetStatusMessage("%s", err)
			e.Filename = ""
		}
		e.syntax = highlighter.SelectSyntaxHighlight(e.Filename)
	}
	var msg string
	msg, e.Dirty = filemgt.Save(e.Filename, e.rowsToString)
	e.SetStatusMessage(msg)
	e.UpdateAllSyntax()
	if e.Dirty {
		e.Filename = ""
	} // Still dirty? File didn't get written.
	return true, nil
}

/*** output ***/

func (e *Editor) scroll() {
	e.rx = 0

	if e.cy < e.numRows {
		e.rx = e.rows[e.cy].RowCxToRx(e.cx)
	}

	if e.cy < e.rowoff {
		e.rowoff = e.cy
	}
	if e.cy >= e.rowoff+e.screenRows {
		e.rowoff = e.cy - e.screenRows + 1
	}
	if e.rx < e.coloff {
		e.coloff = e.rx
	}
	if e.rx >= e.coloff+e.screenCols {
		e.coloff = e.rx - e.screenCols + 1
	}
}

// RefreshScreen resets the entire screen based on the internal
// state of an Editor object, and its internal file representation.
func (e *Editor) RefreshScreen() {
	e.scroll()
	ab := bytes.NewBufferString("\x1b[25l")
	ab.WriteString("\x1b[H")
	e.drawRows(ab)
	e.drawStatusBar(ab)
	e.drawMessageBar(ab)
	ab.WriteString(fmt.Sprintf("\x1b[%d;%dH", (e.cy-e.rowoff)+1, (e.rx-e.coloff)+1))
	ab.WriteString("\x1b[?25h")
	_, err := ab.WriteTo(os.Stdout)
	if err != nil {
		log.Fatal(err)
	}
}

func (e *Editor) padRow(ab *bytes.Buffer) {
	w := fmt.Sprintf("Kilo editor -- version %s", kiloVersion)
	if len(w) > e.screenCols {
		w = w[0:e.screenCols]
	}
	pad := "~ "
	for padding := (e.screenCols - len(w)) / 2; padding > 0; padding-- {
		ab.WriteString(pad)
		pad = " "
	}
	ab.WriteString(w)
}

func (e *Editor) ordinaryRow(filerow int, ab *bytes.Buffer) {
	length := e.rows[filerow].Rsize - e.coloff
	if length < 0 {
		length = 0
	}
	if length > 0 {
		if length > e.screenCols {
			length = e.screenCols
		}
		rindex := e.coloff + length
		rw := e.rows[filerow]
		hl := rw.Hl[e.coloff:rindex]
		currentColor := -1
		for j, c := range e.rows[filerow].Render[e.coloff:rindex] {
			switch {
			case unicode.IsControl(rune(c)):
				ab.WriteString("\x1b[7m")
				if c < 26 {
					ab.WriteString("@")
				} else {
					ab.WriteString("?")
				}
				ab.WriteString("\x1b[m")
				if currentColor != -1 {
					ab.WriteString(fmt.Sprintf("\x1b[%dm", currentColor))
				}
			case hl[j] == highlighter.HL_NORMAL:
				if currentColor != -1 {
					ab.WriteString("\x1b[39m")
					currentColor = -1
				}
				ab.WriteByte(c)
			default:
				color := highlighter.SyntaxToColor(hl[j])
				if color != currentColor {
					currentColor = color
					buf := fmt.Sprintf("\x1b[%dm", color)
					ab.WriteString(buf)
				}
				ab.WriteByte(c)
			}
		}
		ab.WriteString("\x1b[39m")
	}
}

func (e *Editor) drawRows(ab *bytes.Buffer) {
	for y := 0; y < e.screenRows; y++ {
		filerow := y + e.rowoff
		if filerow >= e.numRows {
			if e.numRows == 0 && y == e.screenRows/3 {
				e.padRow(ab)
			} else {
				ab.WriteString("~")
			}
		} else {
			e.ordinaryRow(filerow, ab)
		}
		ab.WriteString("\x1b[K")
		ab.WriteString("\r\n")
	}
}

func (e *Editor) drawStatusBar(ab *bytes.Buffer) {
	ab.WriteString("\x1b[7m")
	fname := e.Filename
	if fname == "" {
		fname = "[No Name]"
	}
	modified := ""
	if e.Dirty {
		modified = "(modified)"
	}
	status := fmt.Sprintf("%.20s - %d lines %s", fname, e.numRows, modified)
	ln := len(status)
	if ln > e.screenCols {
		ln = e.screenCols
	}
	filetype := "no ft"
	if e.syntax != nil {
		filetype = e.syntax.Filetype
	}
	rstatus := fmt.Sprintf("%s | %d/%d", filetype, e.cy+1, e.numRows)
	rlen := len(rstatus)
	ab.WriteString(status[:ln])
	for ln < e.screenCols {
		if e.screenCols-ln == rlen {
			ab.WriteString(rstatus)
			break
		} else {
			ab.WriteString(" ")
			ln++
		}
	}
	ab.WriteString("\x1b[m")
	ab.WriteString("\r\n")
}

func (e *Editor) drawMessageBar(ab *bytes.Buffer) {
	ab.WriteString("\x1b[K")
	msglen := len(e.statusmsg)
	if msglen > e.screenCols {
		msglen = e.screenCols
	}
	if msglen > 0 && (time.Now().Sub(e.statusMsgTime) < 5*time.Second) {
		ab.WriteString(e.statusmsg)
	}
}

// SetStatusMessage invocations should set the text of
// the message the user sees for 5 seconds at the bottom
// of the screen.
func (e *Editor) SetStatusMessage(args ...interface{}) {
	e.statusmsg = fmt.Sprintf(args[0].(string), args[1:]...)
	e.statusMsgTime = time.Now()
}

/*** init ***/

// NewEditor creates an instance of Editor, fresh and ready to go.
func NewEditor() (*Editor, error) {
	var ec Editor
	var e bool
	if ec.screenRows, ec.screenCols, e = screen.GetWindowSize(); !e {
		return nil, fmt.Errorf("couldn't get screen size")
	}
	ec.screenRows -= 2
	return &ec, nil
}
