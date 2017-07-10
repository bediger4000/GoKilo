package main

import (
	"bufio"
	"bytes"
	"fmt"
	"highlighter"
	"io"
	"keyboard"
	"log"
	"os"
	"row"
	"screen"
	"time"
	"tty"
	"unicode"
)

/*** defines ***/

const kiloVersion = "0.0.1"
const kiloQuitTimes = 3

/*** data ***/

type editorConfig struct {
	cx            int
	cy            int
	rx            int
	rowoff        int
	coloff        int
	screenRows    int
	screenCols    int
	numRows       int
	rows          []*row.Row
	dirty         bool
	filename      string
	statusmsg     string
	statusMsgTime time.Time
	syntax        *highlighter.Syntax
}

/*** filetypes ***/

func die(err error) {
	ttyDev.DisableRawMode()
	io.WriteString(os.Stdout, "\x1b[2J")
	io.WriteString(os.Stdout, "\x1b[H")
	log.Fatal(err)
}

/*** syntax hightlighting ***/

func (E *editorConfig) UpdateAllSyntax() {
	inComment := false
	for _, row := range E.rows {
		E.syntax.UpdateSyntax(row, inComment)
		inComment = row.HlOpenComment
	}
}

func (E *editorConfig) UpdateSyntax(at int) {
	inComment := at > 0 && E.rows[at-1].HlOpenComment
	for at < E.numRows && E.syntax.UpdateSyntax(E.rows[at], inComment) {
		at++
		inComment = at > 0 && E.rows[at-1].HlOpenComment
	}
}

func (E *editorConfig) AppendRow(s []byte) {
	E.InsertRow(E.numRows, s)
}

func (E *editorConfig) InsertRow(at int, s []byte) {
	if at < 0 || at > E.numRows {
		return
	}
	var r row.Row
	r.Chars = s
	r.Size = len(s)
	r.Idx = at

	if at == 0 {
		t := make([]*row.Row, 1)
		t[0] = &r
		E.rows = append(t, E.rows...)
	} else if at == E.numRows {
		E.rows = append(E.rows, &r)
	} else {
		t := make([]*row.Row, 1)
		t[0] = &r
		E.rows = append(E.rows[:at], append(t, E.rows[at:]...)...)
	}

	for j := at + 1; j <= E.numRows; j++ {
		E.rows[j].Idx++
	}

	E.rows[at].UpdateRow()
	E.UpdateSyntax(at)
	E.numRows++
	E.dirty = true
}

func (E *editorConfig) DelRow(at int) {
	if at < 0 || at > E.numRows {
		return
	}
	E.rows = append(E.rows[:at], E.rows[at+1:]...)
	E.numRows--
	E.dirty = true
	for j := at; j < E.numRows; j++ {
		E.rows[j].Idx--
	}
}

/*** editor operations ***/

func (E *editorConfig) InsertChar(c byte) {
	if E.cy == E.numRows {
		var emptyRow []byte
		E.AppendRow(emptyRow)
	}
	E.rows[E.cy].RowInsertChar(E.cx, c)
	E.UpdateSyntax(E.cy)
	E.dirty = true
	E.cx++
}

func (E *editorConfig) InsertNewLine() {
	if E.cx == 0 {
		E.InsertRow(E.cy, make([]byte, 0))
	} else {
		E.InsertRow(E.cy+1, E.rows[E.cy].Chars[E.cx:])
		E.rows[E.cy].Chars = E.rows[E.cy].Chars[:E.cx]
		E.rows[E.cy].Size = len(E.rows[E.cy].Chars)
		E.rows[E.cy].UpdateRow()
		E.UpdateSyntax(E.cy)
	}
	E.cy++
	E.cx = 0
}

func (E *editorConfig) DelChar() {
	if E.cy == E.numRows {
		return
	}
	if E.cx == 0 && E.cy == 0 {
		return
	}
	if E.cx > 0 {
		E.rows[E.cy].RowDelChar(E.cx - 1)
		E.UpdateSyntax(E.cy)
		E.cx--
	} else {
		E.cx = E.rows[E.cy-1].Size
		E.rows[E.cy-1].RowAppendString(E.rows[E.cy].Chars)
		E.UpdateSyntax(E.cy - 1)
		E.dirty = true
		E.DelRow(E.cy)
		E.cy--
	}
	E.dirty = true
}

/*** file I/O ***/

func (E *editorConfig) RowsToString() (string, int) {
	totlen := 0
	buf := ""
	for _, row := range E.rows {
		totlen += row.Size + 1
		buf += string(row.Chars) + "\n"
	}
	return buf, totlen
}

func Open(filenames []string, appendF func([]byte)) (filename string) {
	if len(filenames) == 1 {
		return
	}
	filename = filenames[1]
	fd, err := os.Open(filename)
	if err != nil {
		die(err)
	}
	defer fd.Close()
	fp := bufio.NewReader(fd)

	for line, err := fp.ReadBytes('\n'); err == nil; line, err = fp.ReadBytes('\n') {
		// Trim trailing newlines and carriage returns
		for c := line[len(line)-1]; len(line) > 0 && (c == '\n' || c == '\r'); {
			line = line[:len(line)-1]
			if len(line) > 0 {
				c = line[len(line)-1]
			}
		}
		appendF(line)
	}

	if err != nil && err != io.EOF {
		die(err)
	}

	return filename
}

func Save(filename string, getBytes func() (string, int)) (msg string, stillDirty bool) {
	fp, e := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if e != nil {
		return fmt.Sprintf("Can't save! file open error %s", e), true
	}
	defer fp.Close()
	buf, len := getBytes()
	stillDirty = true
	n, err := io.WriteString(fp, buf)
	if err == nil {
		if n == len {
			stillDirty = false
			msg = fmt.Sprintf("%d bytes written to disk", len)
		} else {
			msg = fmt.Sprintf("wanted to write %d bytes to file, wrote %d", len, n)
		}
		return msg, stillDirty
	}
	return fmt.Sprintf("Can't save! I/O error %s", err), stillDirty
}

/*** find ***/

var lastMatch = -1
var direction = 1
var savedHlLine int
var savedHl []byte

func (E *editorConfig) FindCallback(qry []byte, key int) {

	if savedHlLine > 0 {
		copy(E.rows[savedHlLine].Hl, savedHl)
		savedHlLine = 0
		savedHl = nil
	}

	if key == '\r' || key == keyboard.ESCAPE {
		lastMatch = -1
		direction = 1
		return
	} else if key == keyboard.ARROW_RIGHT || key == keyboard.ARROW_DOWN {
		direction = 1
	} else if key == keyboard.ARROW_LEFT || key == keyboard.ARROW_UP {
		direction = -1
	} else {
		lastMatch = -1
		direction = 1
	}

	if lastMatch == -1 {
		direction = 1
	}
	current := lastMatch

	for range E.rows {
		current += direction
		if current == -1 {
			current = E.numRows - 1
		} else if current == E.numRows {
			current = 0
		}
		row := E.rows[current]
		x := bytes.Index(row.Render, qry)
		if x > -1 {
			lastMatch = current
			E.cy = current
			E.cx = row.RowRxToCx(x)
			E.rowoff = E.numRows
			savedHlLine = current
			savedHl = make([]byte, row.Rsize)
			copy(savedHl, row.Hl)
			max := x + len(qry)
			for i := x; i < max; i++ {
				row.Hl[i] = highlighter.HL_MATCH
			}
			break
		}
	}
}

func editorFind(E *editorConfig) {
	savedCx := E.cx
	savedCy := E.cy
	savedColoff := E.coloff
	savedRowoff := E.rowoff
	query := E.Prompt("Search: %s (ESC/Arrows/Enter)",
		E.FindCallback)
	if query == "" {
		E.cx = savedCx
		E.cy = savedCy
		E.coloff = savedColoff
		E.rowoff = savedRowoff
	}
}

/*** input ***/

func (E *editorConfig) Prompt(prompt string, callback func([]byte, int)) string {
	var buf []byte

	for {
		E.SetStatusMessage(prompt, buf)
		E.RefreshScreen()

		c, e := keyboard.ReadKey()
		if e != nil {
			die(e)
		}

		switch c {
		case keyboard.DEL_KEY, keyboard.CTRL_H, keyboard.BACKSPACE:
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
			}
		case keyboard.ESCAPE:
			E.SetStatusMessage("")
			if callback != nil {
				callback(buf, c)
			}
			return ""
		case '\r':
			if len(buf) != 0 {
				E.SetStatusMessage("")
				if callback != nil {
					callback(buf, c)
				}
				return string(buf)
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

func (E *editorConfig) MoveCursor(key int) {
	switch key {
	case keyboard.ARROW_LEFT:
		if E.cx != 0 {
			E.cx--
		} else if E.cy > 0 {
			E.cy--
			E.cx = E.rows[E.cy].Size
		}
	case keyboard.ARROW_RIGHT:
		if E.cy < E.numRows {
			if E.cx < E.rows[E.cy].Size {
				E.cx++
			} else if E.cx == E.rows[E.cy].Size {
				E.cy++
				E.cx = 0
			}
		}
	case keyboard.ARROW_UP:
		if E.cy != 0 {
			E.cy--
		}
	case keyboard.ARROW_DOWN:
		if E.cy < E.numRows {
			E.cy++
		}
	}

	rowlen := 0
	if E.cy < E.numRows {
		rowlen = E.rows[E.cy].Size
	}
	if E.cx > rowlen {
		E.cx = rowlen
	}
}

var quitTimes = kiloQuitTimes

func (E *editorConfig) ProcessKeypress() {
	c, e := keyboard.ReadKey()
	if e != nil {
		die(e)
	}
	switch c {
	case '\r':
		E.InsertNewLine()
		break
	case keyboard.CTRL_Q:
		if E.dirty && quitTimes > 0 {
			E.SetStatusMessage("Warning!!! File has unsaved changes. Press Ctrl-Q %d more times to quit.", quitTimes)
			quitTimes--
			return
		}
		io.WriteString(os.Stdout, "\x1b[2J")
		io.WriteString(os.Stdout, "\x1b[H")
		ttyDev.DisableRawMode()
		os.Exit(0)
	case keyboard.CTRL_S:
		if E.filename == "" {
			E.filename = E.Prompt("Save as: %q", nil)
			if E.filename == "" {
				E.SetStatusMessage("Save aborted")
				return
			}
			E.syntax = highlighter.SelectSyntaxHighlight(E.filename)
		}
		var msg string
		msg, E.dirty = Save(E.filename, E.RowsToString)
		E.SetStatusMessage(msg)
		E.UpdateAllSyntax()
	case keyboard.HOME_KEY:
		E.cx = 0
	case keyboard.END_KEY:
		if E.cy < E.numRows {
			E.cx = E.rows[E.cy].Size
		}
	case keyboard.CTRL_F:
		editorFind(E)
	case keyboard.CTRL_H, keyboard.BACKSPACE, keyboard.DEL_KEY:
		if c == keyboard.DEL_KEY {
			E.MoveCursor(keyboard.ARROW_RIGHT)
		}
		E.DelChar()
		break
	case keyboard.PAGE_UP, keyboard.PAGE_DOWN:
		dir := keyboard.ARROW_DOWN
		if c == keyboard.PAGE_UP {
			E.cy = E.rowoff
			dir = keyboard.ARROW_UP
		} else {
			E.cy = E.rowoff + E.screenRows - 1
			if E.cy > E.numRows {
				E.cy = E.numRows
			}
		}
		for times := E.screenRows; times > 0; times-- {
			E.MoveCursor(dir)
		}
	case keyboard.ARROW_UP, keyboard.ARROW_DOWN,
		keyboard.ARROW_LEFT, keyboard.ARROW_RIGHT:
		E.MoveCursor(c)
	case keyboard.CTRL_L:
		break
	case keyboard.ESCAPE:
		break
	default:
		E.InsertChar(byte(c))
	}
	quitTimes = kiloQuitTimes
}

/*** output ***/

func (E *editorConfig) Scroll() {
	E.rx = 0

	if E.cy < E.numRows {
		E.rx = E.rows[E.cy].RowCxToRx(E.cx)
	}

	if E.cy < E.rowoff {
		E.rowoff = E.cy
	}
	if E.cy >= E.rowoff+E.screenRows {
		E.rowoff = E.cy - E.screenRows + 1
	}
	if E.rx < E.coloff {
		E.coloff = E.rx
	}
	if E.rx >= E.coloff+E.screenCols {
		E.coloff = E.rx - E.screenCols + 1
	}
}

func (E *editorConfig) RefreshScreen() {
	E.Scroll()
	ab := bytes.NewBufferString("\x1b[25l")
	ab.WriteString("\x1b[H")
	E.DrawRows(ab)
	E.DrawStatusBar(ab)
	E.DrawMessageBar(ab)
	ab.WriteString(fmt.Sprintf("\x1b[%d;%dH", (E.cy-E.rowoff)+1, (E.rx-E.coloff)+1))
	ab.WriteString("\x1b[?25h")
	_, e := ab.WriteTo(os.Stdout)
	if e != nil {
		log.Fatal(e)
	}
}

func (E *editorConfig) DrawRows(ab *bytes.Buffer) {
	for y := 0; y < E.screenRows; y++ {
		filerow := y + E.rowoff
		if filerow >= E.numRows {
			if E.numRows == 0 && y == E.screenRows/3 {
				w := fmt.Sprintf("Kilo editor -- version %s", kiloVersion)
				if len(w) > E.screenCols {
					w = w[0:E.screenCols]
				}
				pad := "~ "
				for padding := (E.screenCols - len(w)) / 2; padding > 0; padding-- {
					ab.WriteString(pad)
					pad = " "
				}
				ab.WriteString(w)
			} else {
				ab.WriteString("~")
			}
		} else {
			length := E.rows[filerow].Rsize - E.coloff
			if length < 0 {
				length = 0
			}
			if length > 0 {
				if length > E.screenCols {
					length = E.screenCols
				}
				rindex := E.coloff + length
				rw := E.rows[filerow]
				hl := rw.Hl[E.coloff:rindex]
				currentColor := -1
				for j, c := range E.rows[filerow].Render[E.coloff:rindex] {
					if unicode.IsControl(rune(c)) {
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
					} else if hl[j] == highlighter.HL_NORMAL {
						if currentColor != -1 {
							ab.WriteString("\x1b[39m")
							currentColor = -1
						}
						ab.WriteByte(c)
					} else {
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
		ab.WriteString("\x1b[K")
		ab.WriteString("\r\n")
	}
}

func (E *editorConfig) DrawStatusBar(ab *bytes.Buffer) {
	ab.WriteString("\x1b[7m")
	fname := E.filename
	if fname == "" {
		fname = "[No Name]"
	}
	modified := ""
	if E.dirty {
		modified = "(modified)"
	}
	status := fmt.Sprintf("%.20s - %d lines %s", fname, E.numRows, modified)
	ln := len(status)
	if ln > E.screenCols {
		ln = E.screenCols
	}
	filetype := "no ft"
	if E.syntax != nil {
		filetype = E.syntax.Filetype
	}
	rstatus := fmt.Sprintf("%s | %d/%d", filetype, E.cy+1, E.numRows)
	rlen := len(rstatus)
	ab.WriteString(status[:ln])
	for ln < E.screenCols {
		if E.screenCols-ln == rlen {
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

func (E *editorConfig) DrawMessageBar(ab *bytes.Buffer) {
	ab.WriteString("\x1b[K")
	msglen := len(E.statusmsg)
	if msglen > E.screenCols {
		msglen = E.screenCols
	}
	if msglen > 0 && (time.Now().Sub(E.statusMsgTime) < 5*time.Second) {
		ab.WriteString(E.statusmsg)
	}
}

func (E *editorConfig) SetStatusMessage(args ...interface{}) {
	E.statusmsg = fmt.Sprintf(args[0].(string), args[1:]...)
	E.statusMsgTime = time.Now()
}

/*** init ***/

func initEditor() *editorConfig {
	var ec editorConfig
	var e bool
	if ec.screenRows, ec.screenCols, e = screen.GetWindowSize(); !e {
		die(fmt.Errorf("couldn't get screen size"))
	}
	ec.screenRows -= 2
	return &ec
}

var ttyDev *tty.Tty

func main() {

	ttyDev = new(tty.Tty)
	ttyDev.EnableRawMode()
	defer ttyDev.DisableRawMode()

	E := initEditor()

	E.filename = Open(os.Args, E.AppendRow)
	E.dirty = false
	E.syntax = highlighter.SelectSyntaxHighlight(E.filename)
	E.UpdateAllSyntax()

	E.SetStatusMessage("HELP: Ctrl-S = save | Ctrl-Q = quit | Ctrl-F = find")

	for {
		E.RefreshScreen()
		E.ProcessKeypress()
	}
}
