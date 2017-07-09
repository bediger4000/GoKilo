package highlighter

import (
	"bytes"
	"row"
	"strings"
	"unicode"
)

const (
	hlNORMAL     = 0
	hlCOMMENT    = iota
	hlMLCOMMENT  = iota
	hlKEYWORD1   = iota
	hlKEYWORD2   = iota
	hlSTRING     = iota
	hlNUMBER     = iota
	hlMATCH      = iota
)

type Syntax struct {
	Filetype  string
	filematch []string
	keywords  []string
    singleLineCommentStart []byte
    multiLineCommentStart  []byte
    multiLineCommentEnd    []byte
	flags     int
}

func NormalColored(b byte) bool {
	if b == hlNORMAL {
		return true
	}
	return false
}

func MatchColor() byte {
	return hlMATCH
}

var hldb = []Syntax{
	Syntax{
		Filetype:"c",
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
		flags:hlHIGHLIGHT_NUMBERS|hlHIGHLIGHT_STRINGS,
	},
}

const (
	hlHIGHLIGHT_NUMBERS = 1 << 0
	hlHIGHLIGHT_STRINGS = 1 << iota
)

var separators = []byte(",.()+-/*=~%<>[]; \t\n\r")

func isSeparator(c byte) bool {
	return bytes.IndexByte(separators, c) >= 0
}

func (syntax *Syntax) UpdateSyntax(row *row.Row, inCommentNow bool) (updateNextRow bool) {
	row.Hl = make([]byte, row.Rsize)
	if syntax == nil { return }
	updateNextRow = false
	scs := syntax.singleLineCommentStart
	mcs := syntax.multiLineCommentStart
	mce := syntax.multiLineCommentEnd
	prevSep   := true
	inComment := inCommentNow
	var inString byte
	var skip int
	for i, c := range row.Render {
		if skip > 0 {
			skip--
			continue
		}
		if inString == 0 && len(scs) > 0 && !inComment {
			if bytes.HasPrefix(row.Render[i:], scs) {
				for j := i; j < row.Rsize; j++ {
					row.Hl[j] = hlCOMMENT
				}
				break
			}
		}
		if inString == 0 && len(mcs) > 0 && len(mce) > 0 {
			if inComment {
				row.Hl[i] = hlMLCOMMENT
				if bytes.HasPrefix(row.Render[i:], mce) {
					for l := i; l < i + len(mce); l++ {
						row.Hl[l] = hlMLCOMMENT
					}
					skip = len(mce)
					inComment = false
					prevSep = true
				}
				continue
			} else if bytes.HasPrefix(row.Render[i:], mcs) {
				for l := i; l < i + len(mcs); l++ {
					row.Hl[l] = hlMLCOMMENT
				}
				inComment = true
				skip = len(mcs)
			}
		}
		var prevHl byte = hlNORMAL
		if i > 0 {
			prevHl = row.Hl[i - 1]
		}
		if (syntax.flags & hlHIGHLIGHT_STRINGS) == hlHIGHLIGHT_STRINGS {
			if inString != 0 {
				row.Hl[i] = hlSTRING
				if c == '\\' && i + 1 < row.Rsize {
					row.Hl[i+1] = hlSTRING
					skip = 1
					continue
				}
				if c == inString { inString = 0 }
				prevSep = true
				continue
			} else {
				if c == '"' || c == '\'' {
					inString = c
					row.Hl[i] = hlSTRING
					continue
				}
			}
		}
		if (syntax.flags & hlHIGHLIGHT_NUMBERS) == hlHIGHLIGHT_NUMBERS {
			if unicode.IsDigit(rune(c)) &&
				(prevSep || prevHl == hlNUMBER) ||
				(c == '.' && prevHl == hlNUMBER) {
				row.Hl[i] = hlNUMBER
				prevSep = false
				continue
			}
		}
		if prevSep {
			var j int
			var skw string
			for j, skw = range syntax.keywords[:] {
				kw := []byte(skw)
				var color byte = hlKEYWORD1
				idx := bytes.LastIndexByte(kw, '|')
				if idx > 0 {
					kw = kw[:idx]
					color = hlKEYWORD2
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
			if j < len(syntax.keywords) - 1 {
				prevSep = false
				continue
			}
		}
		prevSep = isSeparator(c)
	}

	updateNextRow = row.HlOpenComment != inComment
	row.HlOpenComment = inComment
	return updateNextRow
}

func SyntaxToColor(hl byte) int {
	switch hl {
	case hlCOMMENT, hlMLCOMMENT:
		return 36
	case hlKEYWORD1:
		return 32
	case hlKEYWORD2:
		return 33
	case hlSTRING:
		return 35
	case hlNUMBER:
		return 31
	case hlMATCH:
		return 34
	}
	return 37
}

func SelectSyntaxHighlight(filename string) (*Syntax) {
	if filename == "" { return nil }

	for _, s := range hldb {
		for _, suffix := range s.filematch {
			if strings.HasSuffix(filename, suffix) {
				return &s
			}
		}
	}
	return nil
}
