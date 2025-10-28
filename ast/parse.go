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

	err    error
	tok    token.Token
	peeked []token.Token
}

func ParseScript(r io.Reader) (*Script, error) {
	clear(loopCnt)
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
	if len(p.peeked) > 0 {
		p.tok, p.peeked = p.peeked[0], p.peeked[1:]
	} else {
		p.tok = p.scan.Next()
	}
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

func (p *parser) peek() token.Token {
	for {
		checkLoop("#peek")
		tok := p.scan.Next()
		p.peeked = append(p.peeked, tok)
		if tok.Type != token.Whitespace {
			return tok
		}
	}
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
		checkLoop("#script")
		var offset bool
		if p.tok.Type == token.Whitespace {
			_, ws, _ := strings.Cut(p.tok.Text, "\n")
			offset = strings.Contains(ws, "\n")
			p.next()
		}
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
	var lastIndent string
	for {
		checkLoop("#stmt")
		// log.Println(p.tok)

		switch p.tok.Type {
		case token.EOF:
			if p.tok.Type != end && end != token.Semicolon {
				p.errorf("unexpected %v, expected %v", p.tok.Type, end)
			}
			p.next()
			return stmt
		case end:
			if end != token.Rparen {
				stmt.nodes = append(stmt.nodes, p.tok)
			}
			p.next()
			return stmt
		case token.Rparen:
			p.errorf("unexpected %v, expected %v", p.tok.Type, end)
			return stmt
		case token.Whitespace:
			if strings.ContainsRune(p.tok.Text, '\n') {

				if i := strings.LastIndexByte(p.tok.Text, '\n'); i >= 0 {
					lastIndent = p.tok.Text[i+1:]
				}

				stmt.nodes = append(stmt.nodes, p.tok)
			}
			p.next()
		default:
			// Indentation should be one level +.
			i := lastIndent + "\t"
			c := p.parseClause(i)
			stmt.nodes = append(stmt.nodes, c)
		}
	}
}

func (p *parser) parseClause(lastIndent string) *Clause {
	c := new(Clause)
	for p.tok.Type == token.Comment {
		checkLoop("#comment")
		c.precede = append(c.precede, p.tok)
		p.next()
		if p.tok.Type != token.Whitespace {
			break
		}
		c.precede = append(c.precede, p.tok)
		p.next()
	}
	var justDeindented bool
	for {
		checkLoop("#clause")
		switch p.tok.Type {
		case token.EOF:
			// Ugly hack so I can sleep.
			if ws, ok := c.nodes[len(c.nodes)-1].(token.Token); ok && ws.Type == token.Whitespace {
				c.nodes = c.nodes[:len(c.nodes)-1]
			}
			fallthrough
		case token.Rparen, token.Semicolon:
			return c
		case token.Lparen:
			p.next()
			sub := p.parseStmt(token.Lparen)
			c.nodes = append(c.nodes, sub)
		case token.Keyword:
			if len(c.nodes) > 0 {
				kword := p.tok.Text
				switch strings.ToUpper(kword) {
				case "ON":
					next := p.peek()
					if strings.ToUpper(next.Text) == "DUPLICATE" {
						return c
					}
				case "LEFT", "RIGHT":
					next := p.peek()
					kword = next.Text
					if next.Type != token.Keyword {
						break
					}
					fallthrough
				default:
					if startsNewClause(kword) {
						return c
					}
				}
			} else {
				c.indentNextLine = true
			}
			fallthrough
		case token.Ident:
			fallthrough
		default:
			switch p.tok.Type {
			case token.Whitespace:
				if i := strings.LastIndexByte(p.tok.Text, '\n'); i >= 0 {
					indent := p.tok.Text[i+1:]
					if strings.HasPrefix(lastIndent, indent) && len(indent) < len(lastIndent) {
						justDeindented = true
					}
					lastIndent = indent
				}
			case token.Comment:
				if justDeindented {
					return c
				}
				fallthrough
			default:
				justDeindented = false
			}
			c.nodes = append(c.nodes, p.tok)
			p.next()
		}
	}
}

func startsNewClause(s string) bool {
	switch strings.ToUpper(s) {
	case "SELECT", "FROM", "JOIN", "WHERE", "HAVING", "GROUP", "ORDER", "LIMIT", "UNION",
		"VALUES", "SET",
		"DUPLICATE":
		return true
	default:
		return false
	}
}

var loopCnt = make(map[string]int)

func checkLoop(name string) {
	loopCnt[name]++
	const limit = 5000
	if loopCnt[name] > limit {
		panic(fmt.Sprint("loop ", name, " is over ", limit, " iterations"))
	}
}
