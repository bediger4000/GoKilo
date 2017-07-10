package highlighter

import (
	"bytes"
	"row"
	"strings"
	"unicode"
)

const (
	HL_NORMAL    = 0
	HL_COMMENT   = iota
	HL_MLCOMMENT = iota
	HL_KEYWORD1  = iota
	HL_KEYWORD2  = iota
	HL_STRING    = iota
	HL_NUMBER    = iota
	HL_MATCH     = iota
)

type Syntax struct {
	Filetype               string
	filematch              []string
	keywords               []string
	singleLineCommentStart []byte
	multiLineCommentStart  []byte
	multiLineCommentEnd    []byte
	flags                  int
}

var hldb = []Syntax{
	{
		Filetype:  "c",
		filematch: []string{".c", ".h", ".cpp"},
		keywords: []string{"switch", "if", "while", "for",
			"break", "continue", "return", "else", "struct",
			"union", "typedef", "static", "enum", "class", "case",
			"int|", "long|", "double|", "float|", "char|",
			"unsigned|", "signed|", "void|",
		},
		singleLineCommentStart: []byte{'/', '/'},
		multiLineCommentStart:  []byte{'/', '*'},
		multiLineCommentEnd:    []byte{'*', '/'},
		flags:                  HL_HIGHLIGHT_NUMBERS | HL_HIGHLIGHT_STRINGS,
	},
	{
		Filetype:  "Go",
		filematch: []string{".go"},
		keywords: []string{"switch", "if", "for", "select",
			"break", "continue", "return", "else", "struct",
			"type", "case", "select", "func", "var", "import",
			"const",
			"int|", "long|", "double|", "float|", "char|", "byte|",
			"unsigned|", "signed|", "string|", "chan|", "bool|",
			"rune|",
		},
		singleLineCommentStart: []byte{'/', '/'},
		multiLineCommentStart:  []byte{'/', '*'},
		multiLineCommentEnd:    []byte{'*', '/'},
		flags:                  HL_HIGHLIGHT_NUMBERS | HL_HIGHLIGHT_STRINGS,
	},
}

const (
	HL_HIGHLIGHT_NUMBERS = 1 << 0
	HL_HIGHLIGHT_STRINGS = 1 << iota
)

var separators = []byte(",.()+-/*=~%<>[]; \t\n\r")

func isSeparator(c byte) bool {
	return bytes.IndexByte(separators, c) >= 0
}

func (syntax *Syntax) UpdateSyntax(row *row.Row, inCommentNow bool) (updateNextRow bool) {
	row.Hl = make([]byte, row.Rsize)
	if syntax == nil {
		return
	}
	updateNextRow = false
	scs := syntax.singleLineCommentStart
	mcs := syntax.multiLineCommentStart
	mce := syntax.multiLineCommentEnd
	prevSep := true
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
					row.Hl[j] = HL_COMMENT
				}
				break
			}
		}
		if inString == 0 && len(mcs) > 0 && len(mce) > 0 {
			if inComment {
				row.Hl[i] = HL_MLCOMMENT
				if bytes.HasPrefix(row.Render[i:], mce) {
					for l := i; l < i+len(mce); l++ {
						row.Hl[l] = HL_MLCOMMENT
					}
					skip = len(mce)
					inComment = false
					prevSep = true
				}
				continue
			} else if bytes.HasPrefix(row.Render[i:], mcs) {
				for l := i; l < i+len(mcs); l++ {
					row.Hl[l] = HL_MLCOMMENT
				}
				inComment = true
				skip = len(mcs)
			}
		}
		var prevHl byte = HL_NORMAL
		if i > 0 {
			prevHl = row.Hl[i-1]
		}
		if (syntax.flags & HL_HIGHLIGHT_STRINGS) == HL_HIGHLIGHT_STRINGS {
			if inString != 0 {
				row.Hl[i] = HL_STRING
				if c == '\\' && i+1 < row.Rsize {
					row.Hl[i+1] = HL_STRING
					skip = 1
					continue
				}
				if c == inString {
					inString = 0
				}
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
		if (syntax.flags & HL_HIGHLIGHT_NUMBERS) == HL_HIGHLIGHT_NUMBERS {
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
			for j, skw = range syntax.keywords[:] {
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
			if j < len(syntax.keywords)-1 {
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

func SelectSyntaxHighlight(filename string) *Syntax {
	if filename == "" {
		return nil
	}

	for _, s := range hldb {
		for _, suffix := range s.filematch {
			if strings.HasSuffix(filename, suffix) {
				return &s
			}
		}
	}
	return nil
}
