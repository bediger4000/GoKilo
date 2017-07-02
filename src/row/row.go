package row

type Row struct {
	Idx           int
	Size          int
	Rsize         int
	Chars         []byte
	Render        []byte
	Hl            []byte
	HlOpenComment bool
}

const KILO_TAB_STOP = 8

func (row *Row) RowCxToRx(cx int) int {
	rx := 0
	for j := 0; j < row.Size && j < cx; j++ {
		if row.Chars[j] == '\t' {
			rx += ((KILO_TAB_STOP - 1) - (rx % KILO_TAB_STOP))
		}
		rx++
	}
	return rx
}

func (row *Row) RowRxToCx(rx int) int {
	curRx := 0
	var cx int
	for cx = 0; cx < row.Size; cx++ {
		if row.Chars[cx] == '\t' {
			curRx += (KILO_TAB_STOP - 1) - (curRx % KILO_TAB_STOP)
		}
		curRx++
		if curRx > rx {
			break
		}
	}
	return cx
}

func (row *Row) UpdateRow() {
	tabs := 0
	for _, c := range row.Chars {
		if c == '\t' {
			tabs++
		}
	}

	row.Render = make([]byte, row.Size+tabs*(KILO_TAB_STOP-1))

	idx := 0
	for _, c := range row.Chars {
		if c == '\t' {
			row.Render[idx] = ' '
			idx++
			for (idx % KILO_TAB_STOP) != 0 {
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
}

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

func (row *Row) RowDelChar(at int) {
	if at < 0 || at > row.Size { return }
	row.Chars = append(row.Chars[:at], row.Chars[at+1:]...)
	row.Size--
	row.UpdateRow()
}

func (row *Row) RowAppendString(s []byte) {
	row.Chars = append(row.Chars, s...)
	row.Size = len(row.Chars)
	row.UpdateRow()
}
