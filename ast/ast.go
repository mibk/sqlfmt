package ast

import "mibk.dev/sqlfmt/token"

// Script is the root AST node representing one or more SQL statements.
// Stmts contains *Stmt and *DelimiterBlock nodes.
type Script struct {
	Stmts []any
}

// DelimiterBlock represents a DELIMITER ... DELIMITER ; block
// whose body is preserved verbatim (not formatted).
type DelimiterBlock struct {
	Open   string // e.g. "DELIMITER //"
	Body   string // raw text between the two DELIMITER commands
	Close  string // e.g. "DELIMITER ;"
	offset bool
}

// Stmt represents a single SQL statement.
type Stmt struct {
	kind   token.Type
	nodes  []any
	offset bool
}

// Clause represents a clause within a statement (e.g. SELECT, FROM, WHERE).
type Clause struct {
	Type           string
	indentNextLine bool
	precede        []token.Token
	nodes          []any
}

// CaseOp represents a CASE ... END expression.
type CaseOp struct {
	nodes []any

	TaggedEnd bool
}

// TypeSpec represents a data type with an optional parenthesized spec
// (e.g. DECIMAL(10, 2)).
type TypeSpec struct {
	Type token.Token
	Spec *Stmt
}
