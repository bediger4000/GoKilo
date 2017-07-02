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
	row.UpdateSyntax()
}

func (row *Row) UpdateSyntax() {
}
