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
	nodes []any
}
