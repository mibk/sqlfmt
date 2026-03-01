package ast

import (
	"cmp"
	"fmt"
	"io"
	"strings"

	"mibk.dev/sqlfmt/token"
)

const (
	magicTokenOffset = 100
	fnCallIdent      = token.Ident + magicTokenOffset
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

	lastIndent     string
	justDeindented bool

	loopCnt map[string]int

	opts
}

type opts struct {
	// alter switches parser to ALTER stmt mode.
	alter bool
}

func ParseScript(r io.Reader) (*Script, error) {
	p := &parser{
		scan:    token.NewScanner(r),
		loopCnt: make(map[string]int),
	}
	p.next() // init
	s := p.parseScript()
	if p.err != nil {
		return nil, p.err
	}
	return s, nil
}

func (p *parser) scanNext() token.Token {
	tok := p.scan.Next()
	if p.alter && tok.Type == token.Ident {
		switch strings.ToUpper(tok.Text) {
		case "MODIFY", "AFTER":
			tok.Type = token.Keyword
		}
	}
	return tok
}

func (p *parser) next() {
	defer func() {
		if p.tok.Type != token.Comment {
			p.justDeindented = false
		}
		if p.tok.Type != token.Whitespace {
			return
		}
		i := strings.LastIndexByte(p.tok.Text, '\n')
		if i < 0 {
			return
		}
		indent := p.tok.Text[i+1:]
		p.justDeindented = strings.HasPrefix(p.lastIndent, indent) &&
			len(indent) < len(p.lastIndent)
		p.lastIndent = indent
	}()
	if p.tok.Type == token.EOF {
		return
	}
	if len(p.peeked) > 0 {
		p.tok, p.peeked = p.peeked[0], p.peeked[1:]
	} else {
		p.tok = p.scanNext()
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
	if len(p.peeked) > 0 {
		return p.peeked[len(p.peeked)-1]
	}
	for {
		p.checkLoop("#peek")
		tok := p.scanNext()
		p.peeked = append(p.peeked, tok)
		if tok.Type != token.Whitespace {
			return tok
		}
	}
}

func (p *parser) skipPeeked() {
	for len(p.peeked) > 0 {
		p.next()
	}
	p.next()
}

func (p *parser) errorf(format string, args ...any) {
	if p.err == nil {
		p.tok.Type = token.EOF
		se := &SyntaxError{Err: fmt.Errorf(format, args...)}
		se.Line, se.Column = p.tok.Pos.Line, p.tok.Pos.Column
		p.err = se
	}
}

func (p *parser) parseScript() *Script {
	s := new(Script)
	for p.tok.Type != token.EOF {
		p.checkLoop("#script")
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
	// Reset parser options.
	backup := p.opts
	p.opts = opts{}
	defer func() { p.opts = backup }()

	end := kind
	if kind == token.Lparen {
		end = token.Rparen
	}
	stmt := new(Stmt)
	stmt.kind = kind
	for {
		p.checkLoop("#stmt")
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
	for p.tok.Type == token.Comment {
		p.checkLoop("#comment")
		c.precede = append(c.precede, p.tok)
		p.next()
		if p.tok.Type != token.Whitespace {
			break
		}
		c.precede = append(c.precede, p.tok)
		p.next()
	}

	// Indentation should be one level +.
	p.lastIndent += "\t"

	fnCallAsKword := true
	for {
		p.checkLoop("#clause")
		switch p.tok.Type {
		case token.EOF:
			if len(c.nodes) == 0 {
				// An even uglier hack so I can sleep.
				if ws := c.precede[len(c.precede)-1]; ws.Type == token.Whitespace {
					c.precede = c.precede[:len(c.precede)-1]
				}
				return c
			}
			// Ugly hack so I can sleep.
			if ws, ok := c.nodes[len(c.nodes)-1].(token.Token); ok && ws.Type == token.Whitespace {
				c.nodes = c.nodes[:len(c.nodes)-1]
			}
			fallthrough
		case token.Rparen, token.Semicolon:
			return c
		case token.Lparen:
			// A hack: backup the indent.
			backup := p.lastIndent

			p.next()
			sub := p.parseStmt(token.Lparen)
			c.nodes = append(c.nodes, sub)

			p.lastIndent = backup
		case token.Keyword:
			kword := strings.ToUpper(p.tok.Text)
			c.Type = cmp.Or(c.Type, kword)
			switch kword {
			case "ALTER":
				p.alter = true
			case "INTO", "CREATE", "ADD", "CHANGE", "CALL",
				"WITH",
				"KEY":
				fnCallAsKword = false
			case "UPDATE":
				fnCallAsKword = true
			case "REPLACE":
				if p.peek().Type == token.Lparen {
					p.tok.Type = token.Ident
					continue
				}
			case "CASE":
				p.next()
				op := p.parseCaseOp()
				c.nodes = append(c.nodes, op)
				continue
			}
			if len(c.nodes) > 0 {
				switch kword {
				case "ON":
					next := p.peek()
					if strings.ToUpper(next.Text) == "DUPLICATE" {
						return c
					}
				case "LEFT", "RIGHT", "INNER", "CROSS":
					next := p.peek()
					kword = next.Text
					if next.Type != token.Keyword {
						p.tok.Type = token.Ident
						break
					}
					fallthrough
				case "MODIFY":
					return c
				default:
					if token.OpensClause(kword) {
						return c
					}
				}
			} else {
				c.indentNextLine = true
			}
			fallthrough
		default:
			c.Type = cmp.Or(c.Type, "<unknown>")
			if p.tok.Type == token.Ident && strings.ToUpper(p.tok.Text) == "ENUM" && p.peek().Type == token.Lparen {
				p.tok.Type = token.DataType
				continue
			} else if p.tok.Type == token.DataType && p.peek().Type == token.Lparen {
				spec := &TypeSpec{Type: p.tok}
				p.next()
				if p.tok.Type == token.Whitespace {
					p.next()
				}
				p.next()
				spec.Spec = p.parseStmt(token.Lparen)
				c.nodes = append(c.nodes, spec)
				continue
			} else if p.tok.Type == token.Ident && fnCallAsKword && p.peek().Type == token.Lparen {
				p.tok.Type = fnCallIdent
			} else if p.tok.Type == token.Comment && p.justDeindented {
				return c
			}
			c.nodes = append(c.nodes, p.tok)
			p.next()
		}
	}
}

func (p *parser) parseCaseOp() *CaseOp {
	c := new(CaseOp)
	for {
		p.checkLoop("#case")
		switch p.tok.Type {
		case token.EOF, token.Semicolon, token.Rparen:
			p.errorf("unexpected %v, expected END", p.tok.Type)
			return c
		case token.Lparen:
			p.next()
			sub := p.parseStmt(token.Lparen)
			c.nodes = append(c.nodes, sub)
		case token.Ident:
			if strings.ToUpper(p.tok.Text) == "END" {
				p.next()
				tok := p.peek()
				if tok.Type == token.Keyword && tok.Text == "CASE" {
					c.TaggedEnd = true
					p.skipPeeked()
				}
				return c
			}
			fallthrough
		default:
			c.nodes = append(c.nodes, p.tok)
			p.next()
		}
	}
}

func (p *parser) checkLoop(name string) {
	p.loopCnt[name]++
	const limit = 5000
	if p.loopCnt[name] > limit {
		panic(fmt.Sprint("loop ", name, " is over ", limit, " iterations"))
	}
}
