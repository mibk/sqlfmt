package ast

import (
	"fmt"

	"mibk.dev/sqlfmt/token"
)

func Simplify(x any) (err error) {
	defer func() {
		v := recover()
		if v != nil {
			err = v.(error)
		}
	}()

	simplify(x)
	return nil
}

func simplify(x any) {
	switch x := x.(type) {
	default:
		panic(fmt.Errorf("ast: unknown type: %T", x))
	case *Script:
		for _, stmt := range x.Stmts {
			simplify(stmt)
		}
	case *Stmt:
		x.nodes = simplifyNodes(x.nodes)
	case *Clause:
		// NOTE: x.precede is just comments.
		x.nodes = simplifyNodes(x.nodes)
	}
}

func simplifyNodes(nodes []any) []any {
	var nn []any
	for _, n := range nodes {
		switch x := n.(type) {
		default:
			simplify(x)
			nn = append(nn, x)
		case token.Token:
			x = simplifyToken(x)
			nn = append(nn, x)
		}
	}
	return nn
}

func simplifyToken(t token.Token) token.Token {
	switch t.Type {
	case token.Quoted:
		if id, ok := token.UnquoteIdent(t.Text); ok {
			t.Text = id
		}
	case token.Neq:
		t.Text = "!="
	}
	return t
}
