package token

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed mariadb.list
var mariadbReservedWords string

var (
	isKeyword   = make(map[string]bool)
	opensClause = make(map[string]bool)
)

func init() {
	for line := range strings.Lines(mariadbReservedWords) {
		if strings.HasPrefix(line, "#") {
			continue
		}

		var ok bool
		kw := strings.TrimSpace(line)
		if strings.HasSuffix(kw, "*") {
			// This one is reserved, but not a keyword.
			continue
		}
		if kw, ok = strings.CutSuffix(kw, "."); ok {
			opensClause[kw] = true
		}
		isKeyword[kw] = true
	}
}

func OpensClause(s string) bool {
	return opensClause[strings.ToUpper(s)]
}

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

type Type uint

const (
	Illegal Type = iota
	EOF
	Whitespace
	Comment

	Keyword
	Ident
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
	Comma     // ,
	Semicolon // ;
	symbolEnd
)
