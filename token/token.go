package token

import (
	"fmt"
	"strings"
)

var isKeyword = make(map[string]bool)

func init() {
	for _, kw := range strings.Fields(mariadbKeywords) {
		isKeyword[kw] = true
	}
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
