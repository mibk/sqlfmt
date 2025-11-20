package ast

import "mibk.dev/sqlfmt/token"

type Script struct {
	Stmts []*Stmt
}

type Stmt struct {
	kind   token.Type
	nodes  []any
	offset bool
}

type Clause struct {
	Type           string
	indentNextLine bool
	precede        []token.Token
	nodes          []any
}

type CaseOp struct {
	nodes []any

	TaggedEnd bool
}

type TypeSpec struct {
	Type token.Token
	Spec *Stmt
}
