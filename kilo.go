package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"keyboard"
	"log"
	"os"
	"screen"
	"strings"
	"time"
	"terminal"
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

type erow struct {
	idx    int
	size   int
	rsize  int
	chars  []byte
	render []byte
	hl     []byte
	hlOpenComment bool
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
	rows        []erow
	dirty       bool
	filename    string
	statusmsg   string
	statusmsg_time time.Time
    syntax      *editorSyntax
	terminal  *terminal.Terminal
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

/*** terminal ***/

func die(err error) {
	E.terminal.DisableRawMode()
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

func editorUpdateSyntax(row *erow) {
	row.hl = make([]byte, row.rsize)
	if E.syntax == nil { return }
	keywords := E.syntax.keywords[:]
	scs := E.syntax.singleLineCommentStart
	mcs := E.syntax.multiLineCommentStart
	mce := E.syntax.multiLineCommentEnd
	prevSep   := true
	inComment := row.idx > 0 && E.rows[row.idx-1].hlOpenComment
	var inString byte = 0
	var skip = 0
	for i, c := range row.render {
		if skip > 0 {
			skip--
			continue
		}
		if inString == 0 && len(scs) > 0 && !inComment {
			if bytes.HasPrefix(row.render[i:], scs) {
				for j := i; j < row.rsize; j++ {
					row.hl[j] = HL_COMMENT
				}
				break
			}
		}
		if inString == 0 && len(mcs) > 0 && len(mce) > 0 {
			if inComment {
				row.hl[i] = HL_MLCOMMENT
				if bytes.HasPrefix(row.render[i:], mce) {
					for l := i; l < i + len(mce); l++ {
						row.hl[l] = HL_MLCOMMENT
					}
					skip = len(mce)
					inComment = false
					prevSep = true
				}
				continue
			} else if bytes.HasPrefix(row.render[i:], mcs) {
				for l := i; l < i + len(mcs); l++ {
					row.hl[l] = HL_MLCOMMENT
				}
				inComment = true
				skip = len(mcs)
			}
		}
		var prevHl byte = HL_NORMAL
		if i > 0 {
			prevHl = row.hl[i - 1]
		}
		if (E.syntax.flags & HL_HIGHLIGHT_STRINGS) == HL_HIGHLIGHT_STRINGS {
			if inString != 0 {
				row.hl[i] = HL_STRING
				if c == '\\' && i + 1 < row.rsize {
					row.hl[i+1] = HL_STRING
					skip = 1
					continue
				}
				if c == inString { inString = 0 }
				prevSep = true
				continue
			} else {
				if c == '"' || c == '\'' {
					inString = c
					row.hl[i] = HL_STRING
					continue
				}
			}
		}
		if (E.syntax.flags & HL_HIGHLIGHT_NUMBERS) == HL_HIGHLIGHT_NUMBERS {
			if unicode.IsDigit(rune(c)) &&
				(prevSep || prevHl == HL_NUMBER) ||
				(c == '.' && prevHl == HL_NUMBER) {
				row.hl[i] = HL_NUMBER
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
				if bytes.HasPrefix(row.render[i:], kw) &&
					(len(row.render[i:]) == klen ||
					isSeparator(row.render[i+klen])) {
					for l := i; l < i+klen; l++ {
						row.hl[l] = color
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

	changed := row.hlOpenComment != inComment
	row.hlOpenComment = inComment
	if changed && row.idx + 1 < E.numRows {
		editorUpdateSyntax(&E.rows[row.idx + 1])
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

func editorRowCxToRx(row *erow, cx int) int {
	rx := 0
	for j := 0; j < row.size && j < cx; j++ {
		if row.chars[j] == '\t' {
			rx += ((KILO_TAB_STOP - 1) - (rx % KILO_TAB_STOP))
		}
		rx++
	}
	return rx
}

func editorRowRxToCx(row *erow, rx int) int {
	curRx := 0
	var cx int
	for cx = 0; cx < row.size; cx++ {
		if row.chars[cx] == '\t' {
			curRx += (KILO_TAB_STOP - 1) - (curRx % KILO_TAB_STOP)
		}
		curRx++
		if curRx > rx { break }
	}
	return cx
}

func editorUpdateRow(row *erow) {
	tabs := 0
	for _, c := range row.chars {
		if c == '\t' {
			tabs++
		}
	}

	row.render = make([]byte, row.size + tabs*(KILO_TAB_STOP - 1))

	idx := 0
	for _, c := range row.chars {
		if c == '\t' {
			row.render[idx] = ' '
			idx++
			for (idx%KILO_TAB_STOP) != 0 {
				row.render[idx] = ' '
				idx++
			}
		} else {
			row.render[idx] = c
			idx++
		}
	}
	row.rsize = idx
	editorUpdateSyntax(row)
}

func editorInsertRow(at int, s []byte) {
	if at < 0 || at > E.numRows { return }
	var r erow
	r.chars = s
	r.size = len(s)
	r.idx = at

	if at == 0 {
		t := make([]erow, 1)
		t[0] = r
		E.rows = append(t, E.rows...)
	} else if at == E.numRows {
		E.rows = append(E.rows, r)
	} else {
		t := make([]erow, 1)
		t[0] = r
		E.rows = append(E.rows[:at], append(t, E.rows[at:]...)...)
	}

	for j := at + 1; j <= E.numRows; j++ { E.rows[j].idx++ }

	editorUpdateRow(&E.rows[at])
	E.numRows++
	E.dirty = true
}

func editorDelRow(at int) {
	if at < 0 || at > E.numRows { return }
	E.rows = append(E.rows[:at], E.rows[at+1:]...)
	E.numRows--
	E.dirty = true
	for j := at; j < E.numRows; j++ { E.rows[j].idx-- }
}

func editorRowInsertChar(row *erow, at int, c byte) {
	if at < 0 || at > row.size {
		row.chars = append(row.chars, c)
	} else if at == 0 {
		t := make([]byte, row.size+1)
		t[0] = c
		copy(t[1:], row.chars)
		row.chars = t
	} else {
		row.chars = append(
			row.chars[:at],
			append(append(make([]byte,0),c), row.chars[at:]...)...
		)
	}
	row.size = len(row.chars)
	editorUpdateRow(row)
	E.dirty = true
}

func editorRowAppendString(row *erow, s []byte) {
	row.chars = append(row.chars, s...)
	row.size = len(row.chars)
	editorUpdateRow(row)
	E.dirty = true
}

func editorRowDelChar(row *erow, at int) {
	if at < 0 || at > row.size { return }
	row.chars = append(row.chars[:at], row.chars[at+1:]...)
	row.size--
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
		editorInsertRow(E.cy+1, E.rows[E.cy].chars[E.cx:])
		E.rows[E.cy].chars = E.rows[E.cy].chars[:E.cx]
		E.rows[E.cy].size = len(E.rows[E.cy].chars)
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
		E.cx = E.rows[E.cy - 1].size
		editorRowAppendString(&E.rows[E.cy - 1], E.rows[E.cy].chars)
		editorDelRow(E.cy)
		E.cy--
	}
}

/*** file I/O ***/

func editorRowsToString() (string, int) {
	totlen := 0
	buf := ""
	for _, row := range E.rows {
		totlen += row.size + 1
		buf += string(row.chars) + "\n"
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
		copy(E.rows[savedHlLine].hl, savedHl)
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
		x := bytes.Index(row.render, qry)
		if x > -1 {
			lastMatch = current
			E.cy = current
			E.cx = editorRowRxToCx(row, x)
			E.rowoff = E.numRows
			savedHlLine = current
			savedHl = make([]byte, row.rsize)
			copy(savedHl, row.hl)
			max := x + len(qry)
			for i := x; i < max; i++ {
				row.hl[i] = HL_MATCH
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
			E.cx = E.rows[E.cy].size
		}
	case keyboard.ARROW_RIGHT:
		if E.cy < E.numRows {
			if E.cx < E.rows[E.cy].size {
				E.cx++
			} else if E.cx == E.rows[E.cy].size {
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
		rowlen = E.rows[E.cy].size
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
		E.terminal.DisableRawMode()
		os.Exit(0)
	case keyboard.CTRL_S:
		editorSave()
	case keyboard.HOME_KEY:
		E.cx = 0
	case keyboard.END_KEY:
		if E.cy < E.numRows {
			E.cx = E.rows[E.cy].size
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
			length := E.rows[filerow].rsize - E.coloff
			if length < 0 { length = 0 }
			if length > 0 {
				if length > E.screenCols { length = E.screenCols }
				rindex := E.coloff+length
				hl := E.rows[filerow].hl[E.coloff:rindex]
				currentColor := -1
				for j, c := range E.rows[filerow].render[E.coloff:rindex] {
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

func main() {

	E.terminal = new(terminal.Terminal)
	E.terminal.EnableRawMode()
	defer E.terminal.DisableRawMode()

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
