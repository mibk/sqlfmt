package ast

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"mibk.dev/sqlfmt/token"
)

// Fprint pretty-prints an AST node to w.
func Fprint(w io.Writer, node any) error {
	tw := tabwriter.NewWriter(w, 0, 0, 1, ' ', tabwriter.StripEscape)

	p := new(printer)
	p.print(node)
	if p.err != nil {
		return p.err
	}

	buf := bufio.NewWriter(tw)
	justIndented := false
	var prevIndentation indentation
	var err error
	for _, tok := range p.tokens {
		if err != nil {
			return err
		}
		switch tok := tok.(type) {
		default:
			err = fmt.Errorf("unsupported type %T", tok)
		case string:
			justIndented = false
			if strings.Contains(tok, "\n") {
				buf.Flush()
				tw.Flush()
			}

			// Normalize line endings to LF.
			tok = strings.ReplaceAll(tok, "\r", "")

			buf.WriteByte(tabwriter.Escape)
			_, err = buf.WriteString(tok)
			buf.WriteByte(tabwriter.Escape)
		case indentation:
			justIndented = true
			if tok != prevIndentation {
				prevIndentation = tok
				buf.Flush()
				tw.Flush()
			}
			buf.WriteByte(tabwriter.Escape)
			for i := 0; i < int(tok); i++ {
				buf.WriteByte('\t')
			}
			err = buf.WriteByte(tabwriter.Escape)
		case whitespace:
			if tok == nextcol {
				tok = space
			}
			if !justIndented || tok == newline {
				err = buf.WriteByte(byte(tok))
			}
		}
	}

	if err := buf.Flush(); err != nil {
		return err
	}
	return tw.Flush()
}

type indentation int

type printer struct {
	indent indentation
	tokens []any
	err    error // sticky

	pflags
}

type pflags struct {
	NoSpaceAfterComma bool
}

type whitespace byte

const (
	nextcol whitespace = '\v'
	newline whitespace = '\n'
	space   whitespace = ' '
	del     whitespace = 'D'
)

func (p *printer) print(args ...any) {
	for _, arg := range args {
		if p.err != nil {
			return
		}

		switch arg := arg.(type) {
		default:
			p.err = fmt.Errorf("unsupported type %T", arg)
		case *Script:
			for _, stmt := range arg.Stmts {
				p.print(stmt)
			}
		case *Stmt:
			if arg.offset {
				p.print(newline)
			}
			parens := arg.kind == token.Lparen
			if parens {
				p.print(token.Lparen)
			}
			allNewlines := arg.hasInterClauseNewline()
			indented := false
			prevNewline := false
			prevSingleKeyword := false
			for i, n := range arg.nodes {
				if parens && !indented && isNewline(n) {
					indented = true
					p.indent++
				} else if c, ok := n.(*Clause); ok && !indented {
					// Looks like a hack to me.
					c.indentNextLine = true
				}
				if _, ok := n.(*Clause); ok && i > 0 && allNewlines && !prevNewline && !prevSingleKeyword {
					p.print(newline, p.indent)
				}
				prevNewline = isNewline(n)
				prevSingleKeyword = false
				if c, ok := n.(*Clause); ok {
					if len(c.nodes) > 0 && isNewline(c.nodes[len(c.nodes)-1]) {
						prevNewline = true
					}
					if isSingleKeyword(c) {
						prevSingleKeyword = true
					}
				}
				p.print(n)
			}
			p.removeLast(space)
			if indented {
				id := p.removeLast(p.indent)
				p.indent--
				if id != nil {
					p.print(p.indent)
				}
			}
			if parens {
				p.print(token.Rparen, space)
			} else {
				p.print(newline)
			}
		case *Clause:
			for _, tok := range arg.precede {
				p.print(tok)
			}
			indented := false
			for _, n := range arg.nodes {
				if arg.indentNextLine && !indented && isNewline(n) {
					indented = true
					p.indent++
				}
				p.print(n)
			}
			if indented {
				id := p.removeLast(p.indent)
				p.indent--
				if id != nil {
					p.print(p.indent)
				}
			}
		case *CaseOp:
			p.print("CASE", space)
			p.indent++
			for _, n := range arg.nodes {
				p.print(n)
			}
			id := p.removeLast(p.indent)
			p.indent--
			if id != nil {
				p.print(p.indent)
			}
			p.print("END", space)
			if arg.TaggedEnd {
				p.print("CASE", space)
			}
		case *TypeSpec:
			backup := p.pflags
			p.NoSpaceAfterComma = true
			p.print(arg.Type, arg.Spec)
			p.pflags = backup
		case token.Token:
			switch arg.Type {
			case token.Whitespace:
				if i := strings.LastIndexByte(arg.Text, '\n'); i >= 0 {
					if strings.Contains(arg.Text[:i], "\n") {
						p.print(newline)
					}
					p.print(newline, p.indent)
				} else {
					p.print(space)
				}
			case fnCallIdent:
				id := strings.ToUpper(arg.Text)
				p.print(id, del)
			case token.Keyword:
				arg.Text = strings.ToUpper(arg.Text)
				p.print(arg.Text, space)
			case token.DataType:
				arg.Text = strings.ToLower(arg.Text)
				p.print(arg.Text)
			default:
				p.print(arg.Text)
			case token.Eq, token.Neq, token.Lt, token.Gt, token.Leq, token.Geq, token.NullEqual:
				p.print(space, arg.Text, space)
			case token.Not:
				p.print(arg.Text, del)
			case token.Comma:
				p.removeLast(space)
				d := space
				if p.NoSpaceAfterComma {
					d = del
				}
				p.print(arg.Text, d)
			case token.Semicolon:
				p.removeLast(space)
				p.print(arg.Text)
			case token.Period:
				p.removeLast(space)
				p.print(arg.Text, del)
			}
		case token.Type:
			if arg == token.EOF {
				break
			}
			p.collect(arg.String())
		case indentation:
			p.collect(arg)
		case whitespace:
			p.collect(arg)
		case string:
			p.collect(arg)
		}
	}
}

func (p *printer) collect(tok any) {
	del := p.removeLast(del)
	switch tok {
	case newline:
		p.removeLast(space)
	case space:
		if del != nil {
			return
		}
		// Prevent double spaces.
		p.removeLast(space)
	}
	p.tokens = append(p.tokens, tok)
}

func (p *printer) removeLast(tok any) any {
	if len(p.tokens) == 0 {
		return nil
	}

	last := p.tokens[len(p.tokens)-1]
	if last == tok {
		p.tokens = p.tokens[:len(p.tokens)-1]
		return last
	}
	if typ, ok := tok.(token.Type); ok {
		if lastTok, ok := last.(token.Token); ok && lastTok.Type == typ {
			p.tokens = p.tokens[:len(p.tokens)-1]
			return last
		}
	}
	return nil
}

// isSingleKeyword reports whether a clause contains only a single
// keyword token, possibly followed by trailing whitespace (like LEFT,
// JOIN, etc.). Such clauses are prefixes that should stay attached
// to the following clause.
func isSingleKeyword(c *Clause) bool {
	n := 0
	for _, node := range c.nodes {
		tok, ok := node.(token.Token)
		if !ok {
			return false
		}
		if tok.Type == token.Whitespace {
			continue
		}
		if tok.Type == token.Keyword && n == 0 {
			n++
			continue
		}
		return false
	}
	return n == 1
}

func isNewline(n any) bool {
	tok, ok := n.(token.Token)
	if !ok {
		return false
	}
	return strings.ContainsRune(tok.Text, '\n')
}

// hasInterClauseNewline reports whether any boundary between
// adjacent clauses contains a newline, ignoring boundaries between
// join prefix clauses (LEFT, RIGHT, etc.) and their following clause.
func (s *Stmt) hasInterClauseNewline() bool {
	clauseCount := 0
	for _, n := range s.nodes {
		if _, ok := n.(*Clause); ok {
			clauseCount++
		}
	}
	if clauseCount < 2 {
		return false
	}
	// Check if any node preceding a clause (after the first) has a
	// newline. The preceding node can be a whitespace token in
	// stmt.nodes or the trailing whitespace inside the prior clause.
	seenFirst := false
	for i, n := range s.nodes {
		if _, ok := n.(*Clause); !ok {
			continue
		}
		if !seenFirst {
			seenFirst = true
			continue
		}
		// Skip boundaries where the previous clause is a single
		// keyword (like LEFT, JOIN) that acts as a prefix.
		prev := s.nodes[i-1]
		if pc, ok := prev.(*Clause); ok && isSingleKeyword(pc) {
			continue
		}
		if isNewline(prev) {
			return true
		}
		// The previous node might be a clause whose last node
		// is trailing whitespace with a newline.
		if pc, ok := prev.(*Clause); ok && len(pc.nodes) > 0 {
			if isNewline(pc.nodes[len(pc.nodes)-1]) {
				return true
			}
		}
	}
	return false
}
