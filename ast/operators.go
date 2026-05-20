package ast

import "mibk.dev/sqlfmt/token"

// preparePrintTree walks the AST and marks each binary arithmetic
// operator according to the loosest-binding op in its expression
// scope, so the printer can pad only the loosest tier and glue
// tighter ops. The expression scope is a Clause (or a CaseOp body);
// each sub-Stmt opens its own scope.
func preparePrintTree(s *Script) {
	for _, stmt := range s.Stmts {
		if st, ok := stmt.(*Stmt); ok {
			prepareStmt(st)
		}
	}
}

func prepareStmt(st *Stmt) {
	for _, n := range st.nodes {
		if c, ok := n.(*Clause); ok {
			prepareScope(c.nodes)
		}
	}
}

// prepareScope analyzes a single expression scope: descends into any
// nested scopes (sub-Stmts, CaseOps, TypeSpecs) and then tightens
// operators in this scope that bind tighter than its loosest one.
func prepareScope(nodes []any) {
	for _, n := range nodes {
		switch n := n.(type) {
		case *Stmt:
			prepareStmt(n)
		case *CaseOp:
			prepareScope(n.nodes)
		case *TypeSpec:
			if n.Spec != nil {
				prepareStmt(n.Spec)
			}
		}
	}

	max := scopeMaxPrec(nodes)
	if max == 0 {
		return
	}
	for i, n := range nodes {
		t, ok := n.(token.Token)
		if !ok {
			continue
		}
		p := binOpPrec(t.Type)
		if p > 0 && p < max {
			t.Type += 2 * magicTokenOffset
			nodes[i] = t
		}
	}
}

// scopeMaxPrec returns the loosest binary-op precedence at the
// surface of nodes, without descending into nested scopes. A return
// of 0 means there are no binary ops at this level.
func scopeMaxPrec(nodes []any) int {
	max := 0
	for _, n := range nodes {
		t, ok := n.(token.Token)
		if !ok {
			continue
		}
		if p := binOpPrec(t.Type); p > max {
			max = p
		}
	}
	return max
}

// binOpPrec returns a precedence number for the binary arithmetic
// and shift operators we space-normalize. Higher numbers bind
// looser. Non-arithmetic tokens return 0.
func binOpPrec(t token.Type) int {
	switch t {
	case token.Mul, token.Quo:
		return 1
	case token.Add, token.Sub:
		return 2
	case token.BitShl, token.BitShr:
		return 3
	}
	return 0
}
