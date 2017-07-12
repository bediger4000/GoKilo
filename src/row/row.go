package row

// Row instances represent a line of text in the file
// under edit.
type Row struct {
	Size          int
	Rsize         int
	Chars         []byte
	Render        []byte
	Hl            []byte
	HlOpenComment bool
}

const kiloTabStop = 8

// RowCxToRx translates "in-file" position in the line to
// rendered position in the line.
func (row *Row) RowCxToRx(cx int) int {
	rx := 0
	for j := 0; j < row.Size && j < cx; j++ {
		if row.Chars[j] == '\t' {
			rx += ((kiloTabStop - 1) - (rx % kiloTabStop))
		}
		rx++
	}
	return rx
}

// RowRxToCx translates rendered position in the line to
// "in-file" position in the line.
func (row *Row) RowRxToCx(rx int) int {
	curRx := 0
	var cx int
	for cx = 0; cx < row.Size; cx++ {
		if row.Chars[cx] == '\t' {
			curRx += (kiloTabStop - 1) - (curRx % kiloTabStop)
		}
		curRx++
		if curRx > rx {
			break
		}
	}
	return cx
}

// UpdateRow creates the "rendered" version of a row, which is the
// bytes displayed on-screen.
func (row *Row) UpdateRow() {
	tabs := 0
	for _, c := range row.Chars {
		if c == '\t' {
			tabs++
		}
	}

	row.Render = make([]byte, row.Size+tabs*(kiloTabStop-1))

	idx := 0
	for _, c := range row.Chars {
		if c == '\t' {
			row.Render[idx] = ' '
			idx++
			for (idx % kiloTabStop) != 0 {
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
	row.Hl = make([]byte, row.Rsize)
}

// RowInsertChar puts byte argument c into a line, position at
func (row *Row) RowInsertChar(at int, c byte) {
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
			append(append(make([]byte, 0), c), row.Chars[at:]...)...,
		)
	}
	row.Size = len(row.Chars)
	row.UpdateRow()
}

// RowDelChar deletes the byte at position at
func (row *Row) RowDelChar(at int) {
	if at < 0 || at > row.Size {
		return
	}
	row.Chars = append(row.Chars[:at], row.Chars[at+1:]...)
	row.Size--
	row.UpdateRow()
}

// RowAppendString adds an array-of-byte to the end of the in-memory
// representation of a text file.
func (row *Row) RowAppendString(s []byte) {
	row.Chars = append(row.Chars, s...)
	row.Size = len(row.Chars)
	row.UpdateRow()
}
