package token

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed mariadb.list
var mariadbReservedWords string

var (
	isReserved  = make(map[string]bool)
	isKeyword   = make(map[string]bool)
	isDataType  = make(map[string]bool)
	opensClause = make(map[string]bool)
)

func init() {
	for line := range strings.Lines(mariadbReservedWords) {
		if strings.HasPrefix(line, "#") {
			continue
		}

		kw := strings.TrimSpace(line)
		var ok bool
		if kw, ok = strings.CutSuffix(kw, "."); ok {
			opensClause[kw] = true
		}

		if kw, ok = strings.CutSuffix(kw, "*"); ok {
			isDataType[kw] = true
		} else if kw, ok = strings.CutSuffix(kw, "("); !ok {
			// This one is reserved, and a keyword.
			isKeyword[kw] = true
		}

		isReserved[kw] = true
	}
}

// OpensClause reports whether the keyword s opens a new SQL clause.
func OpensClause(s string) bool {
	return opensClause[strings.ToUpper(s)]
}

// Token represents a lexical token with its type, text, and position.
type Token struct {
	Type Type
	Text string
	Pos  Pos
}

func (t Token) String() string {
	switch {
	case t.Type == EOF,
		symbolStart < t.Type && t.Type < symbolEnd:
		return t.Type.String()
	default:
		return fmt.Sprintf("%v(%q)", t.Type, t.Text)
	}
}

//go:generate go tool stringer -type Type -linecomment

// Type is the type of a lexical token.
type Type uint

const (
	Illegal Type = iota
	EOF
	Whitespace
	Comment

	Keyword
	Ident
	Quoted
	DataType
	String

	symbolStart
	Lparen    // (
	Rparen    // )
	Period    // .
	Add       // +
	Sub       // -
	Mul       // *
	Quo       // /
	BitShl    // <<
	BitShr    // >>
	Eq        // =
	Neq       // !=
	Lt        // <
	Gt        // >
	Leq       // <=
	Geq       // >=
	NullEqual // <=>
	Not       // !
	Assign    // :=
	Comma     // ,
	Semicolon // ;
	symbolEnd
)

// UnquoteIdent returns the unquoted version of s if possible.
// Otherwise, false is returned as the second return value.
func UnquoteIdent(id string) (u string, ok bool) {
	if len(id) < 3 {
		return "", false
	}

	b, id, e := id[0], id[1:len(id)-1], id[len(id)-1]
	switch b {
	default:
		return "", false
	case '`':
		if e != '`' {
			return "", false
		}
	case '[':
		if e != ']' {
			return "", false
		}
	}

	if isReserved[strings.ToUpper(id)] {
		return "", false
	}
	s := NewScanner(strings.NewReader(id))
	if id != s.scanIdent() {
		return "", false
	}
	return id, true
}
