package ast

import (
	"fmt"
	"slices"
	"strings"

	"mibk.dev/sqlfmt/token"
)

// Simplify applies optional simplifications to the AST: unquoting
// identifiers, removing unnecessary AS, and normalizing <> to !=.
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
		x.nodes = simplifyNodes(x.nodes, false)
	case *Clause:
		// NOTE: x.precede is just comments.
		rewriteAS := false
		switch x.Type {
		case "SELECT", "FROM", "JOIN":
			rewriteAS = true
		}
		x.nodes = simplifyNodes(x.nodes, rewriteAS)
	case *CaseOp:
		x.nodes = simplifyNodes(x.nodes, false)
	case *TypeSpec:
		// There's nothing to simplify here.
	}
}

func simplifyNodes(nodes []any, rewriteAS bool) []any {
	var nn []any
	var toks []token.Token
	flush := func() {
		for _, t := range toks {
			nn = append(nn, t)
		}
		toks = toks[:0]
	}

	for _, n := range nodes {
		switch x := n.(type) {
		default:
			flush()
			simplify(x)
			nn = append(nn, x)
		case token.Token:
			x = simplifyToken(x)
			if rewriteAS && x.Type == token.Ident {
				toks = removeLastAS(toks)
			}
			toks = append(toks, x)
		}
	}

	flush()
	return nn
}

func simplifyToken(t token.Token) token.Token {
	switch t.Type {
	case token.Quoted:
		if id, ok := token.UnquoteIdent(t.Text); ok {
			t.Type = token.Ident
			t.Text = id
		}
	case token.Neq:
		t.Text = "!="
	}
	return t
}

func removeLastAS(toks []token.Token) []token.Token {
	for i, tok := range slices.Backward(toks) {
		if tok.Type == token.Whitespace {
			continue
		}
		if tok.Type == token.Keyword && strings.ToUpper(tok.Text) == "AS" {
			toks = toks[:i]
		}
		break
	}
	return toks
}
