package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"keyboard"
	"log"
	"os"
	"row"
	"screen"
	"strings"
	"time"
	"tty"
	"unicode"
)

/*** defines ***/

const KILO_VERSION = "0.0.1"
const KILO_TAB_STOP = 8
const KILO_QUIT_TIMES = 3

const (
	HL_NORMAL     = 0
	HL_COMMENT    = iota
	HL_MLCOMMENT  = iota
	HL_KEYWORD1   = iota
	HL_KEYWORD2   = iota
	HL_STRING     = iota
	HL_NUMBER     = iota
	HL_MATCH      = iota
)

const (
	HL_HIGHLIGHT_NUMBERS = 1 << 0
	HL_HIGHLIGHT_STRINGS = 1 << iota
)

/*** data ***/

type editorSyntax struct {
	filetype  string
	filematch []string
	keywords  []string
    singleLineCommentStart []byte
    multiLineCommentStart  []byte
    multiLineCommentEnd    []byte
	flags     int
}

type editorConfig struct {
	cx          int
	cy          int
	rx          int
	rowoff      int
	coloff      int
	screenRows  int
	screenCols  int
	numRows     int
	rows        []row.Row
	dirty       bool
	filename    string
	statusmsg   string
	statusmsg_time time.Time
    syntax      *editorSyntax
	tty  *tty.Tty
}

var E editorConfig

/*** filetypes ***/

var HLDB []editorSyntax = []editorSyntax{
	editorSyntax{
		filetype:"c",
		filematch:[]string{".c", ".h", ".cpp"},
		keywords:[]string{"switch", "if", "while", "for",
			"break", "continue", "return", "else", "struct",
			"union", "typedef", "static", "enum", "class", "case",
			"int|", "long|", "double|", "float|", "char|",
			"unsigned|", "signed|", "void|",
		},
		singleLineCommentStart:[]byte{'/', '/'},
		multiLineCommentStart:[]byte{'/', '*'},
		multiLineCommentEnd:[]byte{'*', '/'},
		flags:HL_HIGHLIGHT_NUMBERS|HL_HIGHLIGHT_STRINGS,
	},
}

func die(err error) {
	ttyDev.DisableRawMode()
	io.WriteString(os.Stdout, "\x1b[2J")
	io.WriteString(os.Stdout, "\x1b[H")
	log.Fatal(err)
}

/*** syntax hightlighting ***/
var separators []byte = []byte(",.()+-/*=~%<>[]; \t\n\r")
func isSeparator(c byte) bool {
	if bytes.IndexByte(separators, c) >= 0 {
		return true
	}
	return false
}

func editorUpdateSyntax(row *row.Row) {
	row.Hl = make([]byte, row.Rsize)
	if E.syntax == nil { return }
	keywords := E.syntax.keywords[:]
	scs := E.syntax.singleLineCommentStart
	mcs := E.syntax.multiLineCommentStart
	mce := E.syntax.multiLineCommentEnd
	prevSep   := true
	inComment := row.Idx > 0 && E.rows[row.Idx-1].HlOpenComment
	var inString byte = 0
	var skip = 0
	for i, c := range row.Render {
		if skip > 0 {
			skip--
			continue
		}
		if inString == 0 && len(scs) > 0 && !inComment {
			if bytes.HasPrefix(row.Render[i:], scs) {
				for j := i; j < row.Rsize; j++ {
					row.Hl[j] = HL_COMMENT
				}
				break
			}
		}
		if inString == 0 && len(mcs) > 0 && len(mce) > 0 {
			if inComment {
				row.Hl[i] = HL_MLCOMMENT
				if bytes.HasPrefix(row.Render[i:], mce) {
					for l := i; l < i + len(mce); l++ {
						row.Hl[l] = HL_MLCOMMENT
					}
					skip = len(mce)
					inComment = false
					prevSep = true
				}
				continue
			} else if bytes.HasPrefix(row.Render[i:], mcs) {
				for l := i; l < i + len(mcs); l++ {
					row.Hl[l] = HL_MLCOMMENT
				}
				inComment = true
				skip = len(mcs)
			}
		}
		var prevHl byte = HL_NORMAL
		if i > 0 {
			prevHl = row.Hl[i - 1]
		}
		if (E.syntax.flags & HL_HIGHLIGHT_STRINGS) == HL_HIGHLIGHT_STRINGS {
			if inString != 0 {
				row.Hl[i] = HL_STRING
				if c == '\\' && i + 1 < row.Rsize {
					row.Hl[i+1] = HL_STRING
					skip = 1
					continue
				}
				if c == inString { inString = 0 }
				prevSep = true
				continue
			} else {
				if c == '"' || c == '\'' {
					inString = c
					row.Hl[i] = HL_STRING
					continue
				}
			}
		}
		if (E.syntax.flags & HL_HIGHLIGHT_NUMBERS) == HL_HIGHLIGHT_NUMBERS {
			if unicode.IsDigit(rune(c)) &&
				(prevSep || prevHl == HL_NUMBER) ||
				(c == '.' && prevHl == HL_NUMBER) {
				row.Hl[i] = HL_NUMBER
				prevSep = false
				continue
			}
		}
		if prevSep {
			var j int
			var skw string
			for j, skw = range keywords {
				kw := []byte(skw)
				var color byte = HL_KEYWORD1
				idx := bytes.LastIndexByte(kw, '|')
				if idx > 0 {
					kw = kw[:idx]
					color = HL_KEYWORD2
				}
				klen := len(kw)
				if bytes.HasPrefix(row.Render[i:], kw) &&
					(len(row.Render[i:]) == klen ||
					isSeparator(row.Render[i+klen])) {
					for l := i; l < i+klen; l++ {
						row.Hl[l] = color
					}
					skip = klen - 1
					break
				}
			}
			if j < len(keywords) - 1 {
				prevSep = false
				continue
			}
		}
		prevSep = isSeparator(c)
	}

	changed := row.HlOpenComment != inComment
	row.HlOpenComment = inComment
	if changed && row.Idx + 1 < E.numRows {
		editorUpdateSyntax(&E.rows[row.Idx + 1])
	}
}

func editorSyntaxToColor(hl byte) int {
	switch hl {
	case HL_COMMENT, HL_MLCOMMENT:
		return 36
	case HL_KEYWORD1:
		return 32
	case HL_KEYWORD2:
		return 33
	case HL_STRING:
		return 35
	case HL_NUMBER:
		return 31
	case HL_MATCH:
		return 34
	}
	return 37
}

func editorSelectSyntaxHighlight() {
	if E.filename == "" { return }

	for _, s := range HLDB {
		for _, suffix := range s.filematch {
			if strings.HasSuffix(E.filename, suffix) {
				E.syntax = &s
				return
			}
		}
	}
}

/*** row operations ***/

func editorRowCxToRx(row *row.Row, cx int) int {
	rx := 0
	for j := 0; j < row.Size && j < cx; j++ {
		if row.Chars[j] == '\t' {
			rx += ((KILO_TAB_STOP - 1) - (rx % KILO_TAB_STOP))
		}
		rx++
	}
	return rx
}

func editorRowRxToCx(row *row.Row, rx int) int {
	curRx := 0
	var cx int
	for cx = 0; cx < row.Size; cx++ {
		if row.Chars[cx] == '\t' {
			curRx += (KILO_TAB_STOP - 1) - (curRx % KILO_TAB_STOP)
		}
		curRx++
		if curRx > rx { break }
	}
	return cx
}

func editorUpdateRow(row *row.Row) {
	tabs := 0
	for _, c := range row.Chars {
		if c == '\t' {
			tabs++
		}
	}

	row.Render = make([]byte, row.Size + tabs*(KILO_TAB_STOP - 1))

	idx := 0
	for _, c := range row.Chars {
		if c == '\t' {
			row.Render[idx] = ' '
			idx++
			for (idx%KILO_TAB_STOP) != 0 {
				row.Render[idx] = ' '
				idx++
			}
		} else {
			row.Render[idx] = c
			idx++
		}
	}
	row.Rsize = idx
	row.Render = row.Render[0:idx]
	editorUpdateSyntax(row)
}

func editorInsertRow(at int, s []byte) {
	if at < 0 || at > E.numRows { return }
	var r row.Row
	r.Chars = s
	r.Size = len(s)
	r.Idx = at

	if at == 0 {
		t := make([]row.Row, 1)
		t[0] = r
		E.rows = append(t, E.rows...)
	} else if at == E.numRows {
		E.rows = append(E.rows, r)
	} else {
		t := make([]row.Row, 1)
		t[0] = r
		E.rows = append(E.rows[:at], append(t, E.rows[at:]...)...)
	}

	for j := at + 1; j <= E.numRows; j++ { E.rows[j].Idx++ }

	editorUpdateRow(&E.rows[at])
	E.numRows++
	E.dirty = true
}

func editorDelRow(at int) {
	if at < 0 || at > E.numRows { return }
	E.rows = append(E.rows[:at], E.rows[at+1:]...)
	E.numRows--
	E.dirty = true
	for j := at; j < E.numRows; j++ { E.rows[j].Idx-- }
}

func editorRowInsertChar(row *row.Row, at int, c byte) {
	if at < 0 || at > row.Size {
		row.Chars = append(row.Chars, c)
	} else if at == 0 {
		t := make([]byte, row.Size+1)
		t[0] = c
		copy(t[1:], row.Chars)
		row.Chars = t
	} else {
		row.Chars = append(
			row.Chars[:at],
			append(append(make([]byte,0),c), row.Chars[at:]...)...
		)
	}
	row.Size = len(row.Chars)
	editorUpdateRow(row)
	E.dirty = true
}

func editorRowAppendString(row *row.Row, s []byte) {
	row.Chars = append(row.Chars, s...)
	row.Size = len(row.Chars)
	editorUpdateRow(row)
	E.dirty = true
}

func editorRowDelChar(row *row.Row, at int) {
	if at < 0 || at > row.Size { return }
	row.Chars = append(row.Chars[:at], row.Chars[at+1:]...)
	row.Size--
	E.dirty = true
	editorUpdateRow(row)
}

/*** editor operations ***/

func editorInsertChar(c byte) {
	if E.cy == E.numRows {
		var emptyRow []byte
		editorInsertRow(E.numRows, emptyRow)
	}
	editorRowInsertChar(&E.rows[E.cy], E.cx, c)
	E.cx++
}

func editorInsertNewLine() {
	if E.cx == 0 {
		editorInsertRow(E.cy, make([]byte, 0))
	} else {
		editorInsertRow(E.cy+1, E.rows[E.cy].Chars[E.cx:])
		E.rows[E.cy].Chars = E.rows[E.cy].Chars[:E.cx]
		E.rows[E.cy].Size = len(E.rows[E.cy].Chars)
		editorUpdateRow(&E.rows[E.cy])
	}
	E.cy++
	E.cx = 0
}

func editorDelChar() {
	if E.cy == E.numRows { return }
	if E.cx == 0 && E.cy == 0 { return }
	if E.cx > 0 {
    	editorRowDelChar(&E.rows[E.cy], E.cx - 1)
		E.cx--
	} else {
		E.cx = E.rows[E.cy - 1].Size
		editorRowAppendString(&E.rows[E.cy - 1], E.rows[E.cy].Chars)
		editorDelRow(E.cy)
		E.cy--
	}
}

/*** file I/O ***/

func editorRowsToString() (string, int) {
	totlen := 0
	buf := ""
	for _, row := range E.rows {
		totlen += row.Size + 1
		buf += string(row.Chars) + "\n"
	}
	return buf, totlen
}

func editorOpen(filename string) {
	E.filename = filename
	editorSelectSyntaxHighlight()
	fd, err := os.Open(filename)
	if err != nil {
		die(err)
	}
	defer fd.Close()
	fp := bufio.NewReader(fd)

	for line, err := fp.ReadBytes('\n'); err == nil; line, err = fp.ReadBytes('\n') { 
		// Trim trailing newlines and carriage returns
		for c := line[len(line) - 1]; len(line) > 0 && (c == '\n' || c == '\r'); {
			line = line[:len(line)-1]
			if len(line) > 0 {
				c = line[len(line) - 1]
			}
		}
		editorInsertRow(E.numRows, line)
	}

	if err != nil && err != io.EOF {
		die(err)
	}
	E.dirty = false
}

func editorSave() {
	if E.filename == "" {
		E.filename = editorPrompt("Save as: %q", nil)
		if E.filename == "" {
			editorSetStatusMessage("Save aborted")
			return
		}
		editorSelectSyntaxHighlight()
	}
	buf, len := editorRowsToString()
	fp,e := os.OpenFile(E.filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if e != nil {
		editorSetStatusMessage("Can't save! file open error %s", e)
		return
	}
	defer fp.Close()
	n, err := io.WriteString(fp, buf)
	if err == nil {
		if n == len {
			E.dirty = false
			editorSetStatusMessage("%d bytes written to disk", len)
		} else {
			editorSetStatusMessage(fmt.Sprintf("wanted to write %d bytes to file, wrote %d", len, n))
		}
		return
	}
	editorSetStatusMessage("Can't save! I/O error %s", err)
}

/*** find ***/

var lastMatch int = -1
var direction int = 1
var savedHlLine int
var savedHl []byte

func editorFindCallback(qry []byte, key int) {

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
	} else if key == keyboard.ARROW_LEFT  || key == keyboard.ARROW_UP   {
		direction = -1
	} else {
		lastMatch = -1
		direction = 1
	}

	if lastMatch == -1 { direction = 1 }
	current := lastMatch

	for _ = range E.rows {
		current += direction
		if current == -1 {
			current = E.numRows - 1
		} else if current == E.numRows {
			current = 0
		}
		row := &E.rows[current]
		x := bytes.Index(row.Render, qry)
		if x > -1 {
			lastMatch = current
			E.cy = current
			E.cx = editorRowRxToCx(row, x)
			E.rowoff = E.numRows
			savedHlLine = current
			savedHl = make([]byte, row.Rsize)
			copy(savedHl, row.Hl)
			max := x + len(qry)
			for i := x; i < max; i++ {
				row.Hl[i] = HL_MATCH
			}
			break
		}
	}
}

func editorFind() {
	savedCx     := E.cx
	savedCy     := E.cy
	savedColoff := E.coloff
	savedRowoff := E.rowoff
	query := editorPrompt("Search: %s (ESC/Arrows/Enter)",
		editorFindCallback)
	if query == "" {
		E.cx = savedCx
		E.cy = savedCy
		E.coloff = savedColoff
		E.rowoff = savedRowoff
	}
}

/*** input ***/

func editorPrompt(prompt string, callback func([]byte,int)) string {
	var buf []byte

	for {
		editorSetStatusMessage(prompt, buf)
		editorRefreshScreen()

		c, e := keyboard.ReadKey()
		if e != nil { die(e) }

		switch c {
		case keyboard.DEL_KEY, keyboard.CTRL_H, keyboard.BACKSPACE:
			if (len(buf) > 0) {
				buf = buf[:len(buf)-1]
			}
		case keyboard.ESCAPE:
			editorSetStatusMessage("")
			if callback != nil {
				callback(buf, c)
			}
			return ""
		case '\r':
			if len(buf) != 0 {
				editorSetStatusMessage("")
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

func editorMoveCursor(key int) {
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

var quitTimes int = KILO_QUIT_TIMES

func editorProcessKeypress() {
	c, e := keyboard.ReadKey()
	if e != nil { die(e) }
	switch c {
	case '\r':
		editorInsertNewLine()
		break
	case keyboard.CTRL_Q:
		if E.dirty && quitTimes > 0 {
			editorSetStatusMessage("Warning!!! File has unsaved changes. Press Ctrl-Q %d more times to quit.", quitTimes)
			quitTimes--
			return
		}
		io.WriteString(os.Stdout, "\x1b[2J")
		io.WriteString(os.Stdout, "\x1b[H")
		ttyDev.DisableRawMode()
		os.Exit(0)
	case keyboard.CTRL_S:
		editorSave()
	case keyboard.HOME_KEY:
		E.cx = 0
	case keyboard.END_KEY:
		if E.cy < E.numRows {
			E.cx = E.rows[E.cy].Size
		}
	case keyboard.CTRL_F:
		editorFind()
	case keyboard.CTRL_H, keyboard.BACKSPACE, keyboard.DEL_KEY:
		if c == keyboard.DEL_KEY { editorMoveCursor(keyboard.ARROW_RIGHT) }
		editorDelChar()
		break
	case keyboard.PAGE_UP, keyboard.PAGE_DOWN:
		dir := keyboard.ARROW_DOWN
		if c == keyboard.PAGE_UP {
			E.cy = E.rowoff
			dir = keyboard.ARROW_UP
		} else {
			E.cy = E.rowoff + E.screenRows - 1
			if E.cy > E.numRows { E.cy = E.numRows }
		}
		for times := E.screenRows; times > 0; times-- {
			editorMoveCursor(dir)
		}
	case keyboard.ARROW_UP, keyboard.ARROW_DOWN,
		keyboard.ARROW_LEFT, keyboard.ARROW_RIGHT:
		editorMoveCursor(c)
	case keyboard.CTRL_L:
		break
	case keyboard.ESCAPE:
		break
	default:
		editorInsertChar(byte(c))
	}
	quitTimes = KILO_QUIT_TIMES
}

/*** output ***/

func editorScroll() {
	E.rx = 0

	if (E.cy < E.numRows) {
		E.rx = editorRowCxToRx(&(E.rows[E.cy]), E.cx)
	}

	if E.cy < E.rowoff {
		E.rowoff = E.cy
	}
	if E.cy >= E.rowoff + E.screenRows {
		E.rowoff = E.cy - E.screenRows + 1
	}
	if E.rx < E.coloff {
		E.coloff = E.rx
	}
	if E.rx >= E.coloff + E.screenCols {
		E.coloff = E.rx - E.screenCols + 1
	}
}

func editorRefreshScreen() {
	editorScroll()
	ab := bytes.NewBufferString("\x1b[25l")
	ab.WriteString("\x1b[H")
	editorDrawRows(ab)
	editorDrawStatusBar(ab)
	editorDrawMessageBar(ab)
	ab.WriteString(fmt.Sprintf("\x1b[%d;%dH", (E.cy - E.rowoff) + 1, (E.rx - E.coloff) + 1))
	ab.WriteString("\x1b[?25h")
	_, e := ab.WriteTo(os.Stdout)
	if e != nil {
		log.Fatal(e)
	}
}

func editorDrawRows(ab *bytes.Buffer) {
	for y := 0; y < E.screenRows; y++ {
		filerow := y + E.rowoff
		if filerow >= E.numRows {
			if E.numRows == 0 && y == E.screenRows/3 {
				w := fmt.Sprintf("Kilo editor -- version %s", KILO_VERSION)
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
			if length < 0 { length = 0 }
			if length > 0 {
				if length > E.screenCols { length = E.screenCols }
				rindex := E.coloff+length
				hl := E.rows[filerow].Hl[E.coloff:rindex]
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
					} else if hl[j] == HL_NORMAL {
						if currentColor != -1 {
							ab.WriteString("\x1b[39m")
							currentColor = -1
						}
						ab.WriteByte(c)
					} else {
						color := editorSyntaxToColor(hl[j])
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

func editorDrawStatusBar(ab *bytes.Buffer) {
	ab.WriteString("\x1b[7m")
	fname := E.filename
	if fname == "" {
		fname = "[No Name]"
	}
	modified := ""
	if E.dirty { modified = "(modified)" }
	status := fmt.Sprintf("%.20s - %d lines %s", fname, E.numRows, modified)
	ln := len(status)
	if ln > E.screenCols { ln = E.screenCols }
	filetype := "no ft"
	if E.syntax != nil {
		filetype = E.syntax.filetype
	}
	rstatus := fmt.Sprintf("%s | %d/%d", filetype, E.cy+1, E.numRows)
	rlen := len(rstatus)
	ab.WriteString(status[:ln])
	for ln < E.screenCols {
		if E.screenCols - ln == rlen {
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

func editorDrawMessageBar(ab *bytes.Buffer) {
	ab.WriteString("\x1b[K")
	msglen := len(E.statusmsg)
	if msglen > E.screenCols { msglen = E.screenCols }
	if msglen > 0 && (time.Now().Sub(E.statusmsg_time) < 5*time.Second) {
		ab.WriteString(E.statusmsg)
	}
}

func editorSetStatusMessage(args...interface{}) {
	E.statusmsg = fmt.Sprintf(args[0].(string), args[1:]...)
	E.statusmsg_time = time.Now()
}

/*** init ***/

func initEditor() {
	// Initialization a la C not necessary.
	var e bool
	if E.screenRows, E.screenCols, e = screen.GetWindowSize();  !e {
		die(fmt.Errorf("couldn't get screen size"))
	}
	E.screenRows -= 2
}

var ttyDev *tty.Tty

func main() {

	ttyDev = new(tty.Tty)
	ttyDev.EnableRawMode()
	defer ttyDev.DisableRawMode()

	initEditor()

	if len(os.Args) > 1 {
		editorOpen(os.Args[1])
	}

	editorSetStatusMessage("HELP: Ctrl-S = save | Ctrl-Q = quit | Ctrl-F = find")

	for {
		editorRefreshScreen()
		editorProcessKeypress()
	}
}
