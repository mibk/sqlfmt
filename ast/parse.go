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

	// Disambiguated forms of +, -, * (set by the parser based on
	// surrounding tokens, since the scanner can't tell unary from
	// binary or wildcard from multiplication).
	unaryAdd = token.Add + magicTokenOffset // +x
	unarySub = token.Sub + magicTokenOffset // -x
	wildcard = token.Mul + magicTokenOffset // SELECT *, t.*

	// Tight forms of binary operators — printed without surrounding
	// spaces when the precedence pass decides they bind tighter than
	// the loosest operator in their expression.
	tightAdd    = token.Add + 2*magicTokenOffset
	tightSub    = token.Sub + 2*magicTokenOffset
	tightMul    = token.Mul + 2*magicTokenOffset
	tightQuo    = token.Quo + 2*magicTokenOffset
	tightBitShl = token.BitShl + 2*magicTokenOffset
	tightBitShr = token.BitShr + 2*magicTokenOffset
)

// noPrev is a sentinel for "start of expression"; chosen so it can't
// collide with any token.Type value (token.Illegal happens to be 0,
// and unquoted numeric literals are scanned as Illegal).
const noPrev token.Type = 0xffff

// isUnaryContext reports whether a + or - immediately following a
// token of type prev should be treated as a unary prefix rather than
// a binary operator.
func isUnaryContext(prev token.Type) bool {
	switch prev {
	case noPrev,
		token.Lparen, token.Comma,
		token.Add, token.Sub, token.Mul, token.Quo,
		token.BitShl, token.BitShr,
		token.Eq, token.Neq, token.Lt, token.Gt, token.Leq, token.Geq, token.NullEqual,
		token.Not, token.Assign,
		token.Keyword,
		unaryAdd, unarySub:
		return true
	}
	return false
}

// isWildcardContext reports whether a * immediately following a
// token of type prev is the SQL wildcard (SELECT *, t.*, COUNT(*))
// rather than the multiplication operator.
func isWildcardContext(prev token.Type) bool {
	switch prev {
	case noPrev, token.Period, token.Lparen, token.Comma, token.Keyword:
		return true
	}
	return false
}

// classifyArithOp remaps an Add/Sub/Mul token to its unary or
// wildcard form when the preceding context calls for it.
func classifyArithOp(prev token.Type, tok token.Token) token.Token {
	switch tok.Type {
	case token.Add, token.Sub:
		if isUnaryContext(prev) {
			tok.Type += magicTokenOffset
		}
	case token.Mul:
		if isWildcardContext(prev) {
			tok.Type += magicTokenOffset
		}
	}
	return tok
}

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

	opts
}

// progress fingerprints the parser's observable state; a loop that
// iterates twice in a row without changing it is stuck (missing
// p.next() somewhere), so checkProgress panics. Catches the same
// bug the old iteration-count limit caught, but independently of
// input size.
type progress struct {
	pos    token.Pos
	typ    token.Type
	peeked int
	set    bool
}

func (p *parser) checkProgress(prev *progress, name string) {
	cur := progress{
		pos:    p.tok.Pos,
		typ:    p.tok.Type,
		peeked: len(p.peeked),
		set:    true,
	}
	if prev.set && cur == *prev {
		panic(fmt.Sprintf("parser loop %s stuck at %v (no forward progress)",
			name, p.tok.Pos))
	}
	*prev = cur
}

type opts struct {
	// alter switches parser to ALTER stmt mode.
	alter bool
}

// ParseScript parses SQL source from r and returns the AST.
func ParseScript(r io.Reader) (*Script, error) {
	p := &parser{
		scan: token.NewScanner(r),
	}
	p.next() // init
	s := p.parseScript()
	if p.err != nil {
		return nil, p.err
	}
	preparePrintTree(s)
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
	var prog progress
	for {
		p.checkProgress(&prog, "#peek")
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
	var prog progress
	for p.tok.Type != token.EOF {
		p.checkProgress(&prog, "#script")
		var offset bool
		if p.tok.Type == token.Whitespace {
			_, ws, _ := strings.Cut(p.tok.Text, "\n")
			offset = strings.Contains(ws, "\n")
			p.next()
		}
		if p.tok.Type == token.Ident && strings.EqualFold(p.tok.Text, "DELIMITER") {
			db := p.parseDelimiterBlock()
			db.offset = offset
			s.Stmts = append(s.Stmts, db)
			continue
		}
		stmt := p.parseStmt(token.Semicolon)
		stmt.offset = offset
		if len(stmt.nodes) > 0 {
			s.Stmts = append(s.Stmts, stmt)
		}
	}
	return s
}

func (p *parser) parseDelimiterBlock() *DelimiterBlock {
	kword := p.tok.Text // preserve original case
	rest := p.scan.ScanRestOfLine()
	delimStr := strings.TrimSpace(rest)
	open := kword + " " + delimStr

	body, closeLine := p.scan.ScanRawUntil("DELIMITER")
	close := strings.TrimRight(closeLine, "\r\n")

	p.next() // resync parser token state
	return &DelimiterBlock{
		Open:  open,
		Body:  body,
		Close: close,
	}
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
	var prog progress
	for {
		p.checkProgress(&prog, "#stmt")
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
	var commentProg progress
	for p.tok.Type == token.Comment {
		p.checkProgress(&commentProg, "#comment")
		c.precede = append(c.precede, p.tok)
		p.next()
		if p.tok.Type != token.Whitespace {
			break
		}
		ws := p.tok
		p.next()
		if p.tok.Type == token.EOF {
			break
		}
		c.precede = append(c.precede, ws)
	}

	// Indentation should be one level +.
	p.lastIndent += "\t"

	fnCallAsKword := true
	lastNonWS := noPrev
	var prog progress
	for {
		p.checkProgress(&prog, "#clause")
		switch p.tok.Type {
		case token.EOF, token.Rparen, token.Semicolon:
			return c
		case token.Lparen:
			// Save and restore indent level across subquery.
			backup := p.lastIndent

			p.next()
			sub := p.parseStmt(token.Lparen)
			c.nodes = append(c.nodes, sub)
			lastNonWS = token.Rparen

			// If the subquery ended at a line whose indent is shallower
			// than our clause's continuation level, the input actually
			// dedented past us (e.g. the ) of a CTE sits at column 0).
			// Trust the scanner in that case; otherwise the next clause
			// would see a phantom deindent and break.
			if len(p.lastIndent) >= len(backup) {
				p.lastIndent = backup
			}
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
			case "REPLACE", "DATABASE", "SCHEMA":
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
						// DUPLICATE is not a reserved word in MariaDB,
						// so the scanner emits it as an identifier.
						// Together with the preceding ON it forms the
						// ON DUPLICATE KEY UPDATE clause opener, so
						// promote it here to get uppercase output.
						p.peeked[len(p.peeked)-1].Type = token.Keyword
						return c
					}
				case "STRAIGHT_JOIN":
					// STRAIGHT_JOIN can be either a join type
					// (FROM t1 STRAIGHT_JOIN t2) or a SELECT modifier
					// (SELECT STRAIGHT_JOIN ...). Only open a new
					// clause in the former case.
					if c.Type != "SELECT" {
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
			} else if kword != "WITH" {
				// WITH introduces a comma-separated list of CTEs
				// that should all align with WITH itself, not be
				// indented as continuation lines.
				c.indentNextLine = true
			}
			fallthrough
		default:
			if p.tok.Type == token.Whitespace {
				ws := p.tok
				p.next()
				if p.tok.Type == token.EOF {
					continue
				}
				c.nodes = append(c.nodes, ws)
				continue
			}
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
				lastNonWS = token.Rparen
				continue
			} else if p.tok.Type == token.Ident && fnCallAsKword && p.peek().Type == token.Lparen {
				p.tok.Type = fnCallIdent
			} else if p.tok.Type == token.Comment && p.justDeindented {
				return c
			}
			tok := classifyArithOp(lastNonWS, p.tok)
			c.nodes = append(c.nodes, tok)
			lastNonWS = tok.Type
			p.next()
		}
	}
}

func (p *parser) parseCaseOp() *CaseOp {
	c := new(CaseOp)
	lastNonWS := noPrev
	var prog progress
	for {
		p.checkProgress(&prog, "#case")
		switch p.tok.Type {
		case token.EOF, token.Semicolon, token.Rparen:
			p.errorf("unexpected %v, expected END", p.tok.Type)
			return c
		case token.Lparen:
			p.next()
			sub := p.parseStmt(token.Lparen)
			c.nodes = append(c.nodes, sub)
			lastNonWS = token.Rparen
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
			tok := classifyArithOp(lastNonWS, p.tok)
			c.nodes = append(c.nodes, tok)
			if tok.Type != token.Whitespace {
				lastNonWS = tok.Type
			}
			p.next()
		}
	}
}

