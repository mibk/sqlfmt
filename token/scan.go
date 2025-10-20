package token

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

type ScanError struct {
	Pos Pos
	Err error
}

func (e *ScanError) Error() string {
	return fmt.Sprintf("line:%v: %v", e.Pos, e.Err)
}

type Pos struct {
	Line, Column int
}

func (p Pos) String() string {
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

const eof = -1

type Scanner struct {
	r    *bufio.Reader
	done bool
	err  error

	line, col   int
	lastLineLen int
	lastToken   Type
}

func NewScanner(r io.Reader) *Scanner {
	return &Scanner{
		r:    bufio.NewReader(r),
		line: 1,
		col:  1,
	}
}

func (s *Scanner) Next() (tok Token) {
	pos := s.pos()
	tok = s.scanAny()
	if typ := tok.Type; tok.Text == "" && symbolStart < typ && typ < symbolEnd {
		tok.Text = typ.String()
	}
	tok.Pos = pos
	if tok.Type != Whitespace {
		s.lastToken = tok.Type
	}
	return tok
}

func (s *Scanner) Err() error { return s.err }

func (s *Scanner) errorf(format string, args ...interface{}) Token {
	if s.err == nil {
		s.err = &ScanError{s.pos(), fmt.Errorf(format, args...)}
	}
	return Token{Type: EOF}
}

func (s *Scanner) pos() Pos { return Pos{Line: s.line, Column: s.col} }

func (s *Scanner) read() rune {
	if s.done {
		return eof
	}
	r, _, err := s.r.ReadRune()
	if err != nil {
		if err != io.EOF {
			s.err = err
		}
		s.done = true
		return eof
	}
	if r == '\n' {
		s.line++
		s.lastLineLen, s.col = s.col, 1
	} else {
		s.col++
	}
	return r
}

func (s *Scanner) unread() {
	if s.done {
		return
	}
	if err := s.r.UnreadRune(); err != nil {
		// UnreadRune returns an error only on invalid use.
		panic(err)
	}
	s.col--
	if s.col == 0 {
		s.col = s.lastLineLen
		s.line--
	}
}

func (s *Scanner) peek() rune {
	r := s.read()
	s.unread()
	return r
}

func (s *Scanner) scanAny() (tok Token) {
	switch r := s.read(); r {
	case eof:
		return Token{Type: EOF}
	case '/':
		switch s.read() {
		case '/':
			return s.scanLineComment("//")
		case '*':
			return s.scanBlockComment()
		default:
			s.unread()
			return Token{Type: Quo}
		}
	case '#':
		return s.scanLineComment("#")
	case '(':
		return Token{Type: Lparen}
	case ')':
		return Token{Type: Rparen}
	case '=':
		return Token{Type: Eq}
	case '<':
		switch r2 := s.peek(); r2 {
		case '>':
			s.read()
			return Token{Type: Neq, Text: "<>"}
		case '=':
			s.read()
			if s.peek() == '>' {
				s.read()
				return Token{Type: NullEqual}
			}
			return Token{Type: Leq}
		default:
			return Token{Type: Lt}
		}
	case '>':
		switch r2 := s.peek(); r2 {
		case '=':
			s.read()
			return Token{Type: Geq}
		}
		return Token{Type: Gt}
	case '!':
		switch r2 := s.peek(); r2 {
		case '=':
			s.read()
			return Token{Type: Neq}
		}
		return Token{Type: Geq}
	case '*':
		return Token{Type: Ident, Text: "*"}
	case '.':
		return Token{Type: Period}
	case ',':
		return Token{Type: Comma}
	case ';':
		return Token{Type: Semicolon}
	case ' ', '\t', '\r', '\n':
		s.unread()
		return s.scanWhitespace()
	case '\'':
		return s.scanSingleQuoted()
	case '`':
		// TODO: Implement escaping for this one.
		id := s.scanIdentUntil(r)
		return Token{Type: Ident, Text: string(r) + id}
	case '[':
		id := s.scanIdentUntil(']')
		return Token{Type: Ident, Text: string(r) + id}
	case '$', '%', '?':
		id := s.scanIdent()
		return Token{Type: Ident, Text: string(r) + id}
	default:
		s.unread()
		if id := s.scanIdent(); id != "" {
			t := Token{Type: Ident, Text: id}
			if s.lastToken != Period && isKeyword[strings.ToUpper(id)] {
				t.Type = Keyword
			}
			return t
		}
		s.read()
		return Token{Type: Illegal, Text: string(r)}
	}
}

func (s *Scanner) scanLineComment(start string) Token {
	var b strings.Builder
	for {
		switch r := s.read(); r {
		default:
			b.WriteRune(r)
		case '\n', eof:
			s.unread()
			return Token{Type: Comment, Text: start + b.String()}
		}
	}
}

func (s *Scanner) scanBlockComment() Token {
	var b strings.Builder
	for {
		switch r := s.read(); {
		default:
			b.WriteRune(r)
		case r == '*' && s.peek() == '/':
			s.read()
			return Token{Type: Comment, Text: "/*" + b.String() + "*/"}
		case r == eof:
			return s.errorf("unterminated block comment")
		}
	}
}

func (s *Scanner) scanIdent() string {
	var b strings.Builder
	for {
		switch r := s.read(); {
		case r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= utf8.RuneSelf:
			b.WriteRune(r)
		case isDigit(r):
			if b.Len() > 0 {
				b.WriteRune(r)
				continue
			}
			fallthrough
		default:
			s.unread()
			return b.String()
		}
	}
}

func (s *Scanner) scanWhitespace() Token {
	var b strings.Builder
	for {
		switch r := s.read(); r {
		case ' ', '\t', '\r', '\n':
			b.WriteRune(r)
		default:
			s.unread()
			return Token{Type: Whitespace, Text: b.String()}
		}
	}
}

func (s *Scanner) scanSingleQuoted() Token {
	var b strings.Builder
	for {
		r := s.read()
		b.WriteRune(r)
		switch r {
		case '\\':
			b.WriteRune(s.read())
		case '\'':
			return Token{Type: String, Text: "'" + b.String()}
		case eof:
			return s.errorf("string not terminated")
		}
	}
}

func (s *Scanner) scanIdentUntil(delim rune) string {
	var b strings.Builder
	for {
		r := s.read()
		b.WriteRune(r)
		switch r {
		case delim:
			return b.String()
		case eof:
			s.errorf("ident not terminated")
			return ""
		}
	}
}

func isDigit(r rune) bool { return '0' <= r && r <= '9' }
