package ast

import (
	"fmt"
	"io"
	"log"
	"strings"

	"mibk.dev/sqlfmt/token"
)

// SyntaxError records an error and the position it occurred on.
type SyntaxError struct {
	Line, Column int
	Err          error
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("line:%d:%d: %v", e.Line, e.Column, e.Err)
}

type parser struct {
	scan *token.Scanner

	err  error
	tok  token.Token
	prev token.Token
	alt  *token.Token // on backup
}

func ParseScript(r io.Reader) (*Script, error) {
	p := &parser{scan: token.NewScanner(r)}
	p.next() // init
	s := p.parseScript()
	if p.err != nil {
		return nil, p.err
	}
	return s, nil
}

func (p *parser) next() {
	if p.tok.Type == token.EOF {
		return
	}
	if p.alt != nil {
		p.tok, p.alt = *p.alt, nil
		return
	}
	p.tok = p.scan.Next()
	if p.tok.Type == token.EOF && p.err == nil {
		err := p.scan.Err()
		if se, ok := err.(*token.ScanError); ok {
			// Make sure we always return *SyntaxError.
			p.err = &SyntaxError{
				Line:   se.Pos.Line,
				Column: se.Pos.Column,
				Err:    se.Err,
			}
		} else if err != nil {
			p.errorf("scan: %v", err)
		}
	}
}

func (p *parser) got(typ token.Type) bool {
	if p.tok.Type == typ {
		p.next()
		return true
	}
	return false
}

func (p *parser) errorf(format string, args ...interface{}) {
	if p.err == nil {
		p.tok.Type = token.EOF
		se := &SyntaxError{Err: fmt.Errorf(format, args...)}
		se.Line, se.Column = p.tok.Pos.Line, p.tok.Pos.Column
		p.err = se
	}
}

var _ = log.Println

func (p *parser) parseScript() *Script {
	s := new(Script)
	for p.tok.Type != token.EOF {
		offset := p.got(token.Whitespace)
		stmt := p.parseStmt(token.Semicolon)
		stmt.offset = offset
		if len(stmt.nodes) > 0 {
			s.Stmts = append(s.Stmts, stmt)
		}
	}
	return s
}

func (p *parser) parseStmt(kind token.Type) *Stmt {
	end := kind
	if kind == token.Lparen {
		end = token.Rparen
	}
	stmt := new(Stmt)
	stmt.kind = kind
	for {
		// log.Println(p.tok)

		switch p.tok.Type {
		case token.EOF, end:
			p.next()
			return stmt
		case token.Whitespace:
			if strings.ContainsRune(p.tok.Text, '\n') {
				stmt.nodes = append(stmt.nodes, p.tok)
			}
			p.next()
		default:
			c := p.parseClause()
			stmt.nodes = append(stmt.nodes, c)
		}
	}
}

func (p *parser) parseClause() *Clause {
	c := new(Clause)
	for {
		switch p.tok.Type {
		case token.EOF, token.Rparen, token.Semicolon:
			return c
		case token.Lparen:
			p.next()
			sub := p.parseStmt(token.Lparen)
			c.nodes = append(c.nodes, sub)
		case token.Keyword:
			if len(c.nodes) > 0 && startsNewClause(p.tok.Text) {
				return c
			}
			fallthrough
		case token.Ident:
			fallthrough
		default:
			c.nodes = append(c.nodes, p.tok)
			p.next()
		}
	}
}

func startsNewClause(s string) bool {
	switch strings.ToUpper(s) {
	case "SELECT", "FROM", "JOIN", "WHERE", "ORDER":
		return true
	default:
		return false
	}
}
