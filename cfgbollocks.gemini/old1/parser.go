package cfgbollocks

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Entry represents a single key-value pair.
// Order is preserved by storing these in a slice (list).
type Entry struct {
	Key   string
	Value string
}

// ParseError provides detailed context for why parsing failed.
type ParseError struct {
	Line  int
	Msg   string
	Fatal bool
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
}

// Parser holds the state of the current parsing operation.
type Parser struct {
	scanner *bufio.Reader
	line    int
	entries []Entry
}

func NewParser(r io.Reader) *Parser {
	return &Parser{
		scanner: bufio.NewReader(r),
		line:    1,
	}
}

// Parse executes the parsing logic. 
// In Shot 1, this handles the bootstrap and standard entries using fixed rules.
func (p *Parser) Parse() ([]Entry, error) {
	// 1. Mandatory Bootstrap Entry
	if err := p.parseBootstrap(); err != nil {
		return nil, err
	}

	// 2. Subsequent entries
	for {
		// Peek to check for EOF
		if _, err := p.scanner.Peek(1); err == io.EOF {
			break
		}

		entry, err := p.parseEntry()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		p.entries = append(p.entries, entry)
	}

	return p.entries, nil
}

// parseBootstrap ensures the file starts exactly with "cfgbollocks ~"
func (p *Parser) parseBootstrap() error {
	const prefix = "cfgbollocks"
	
	// Read the first line manually to ensure no leading whitespace
	line, err := p.readLogicalLine()
	if err != nil {
		return &ParseError{Line: p.line, Msg: "file empty or unreadable"}
	}

	if !strings.HasPrefix(line, prefix) {
		return &ParseError{Line: 1, Msg: "file must begin with 'cfgbollocks' (no leading whitespace)"}
	}

	// Check separator
	rem := strings.TrimPrefix(line, prefix)
	rem = strings.TrimSpace(rem)
	if !strings.HasPrefix(rem, "~") {
		return &ParseError{Line: 1, Msg: "expected '~' separator after cfgbollocks"}
	}

	// Get Delimiter
	delim := strings.TrimSpace(strings.TrimPrefix(rem, "~"))
	if delim == "" {
		return &ParseError{Line: 1, Msg: "cfgbollocks requires a delimiter"}
	}

	val, err := p.readHereDoc(delim)
	if err != nil {
		return err
	}

	p.entries = append(p.entries, Entry{Key: prefix, Value: val})
	return nil
}

// parseEntry handles standard entries using the bootstrap rules (fixed for Shot 1)
func (p *Parser) parseEntry() (Entry, error) {
	rawLine, err := p.readLogicalLine()
	if err != nil {
		return Entry{}, err
	}

	// Allow leading whitespace between records (per our discussion)
	line := strings.TrimSpace(rawLine)
	if line == "" {
		// Recurse to skip empty lines
		return p.parseEntry()
	}

	parts := strings.SplitN(line, "~", 2)
	if len(parts) < 2 {
		return Entry{}, &ParseError{Line: p.line - 1, Msg: "missing '~' separator"}
	}

	key := strings.TrimSpace(parts[0])
	delim := strings.TrimSpace(parts[1])

	val, err := p.readHereDoc(delim)
	if err != nil {
		return Entry{}, err
	}

	return Entry{Key: key, Value: val}, nil
}

// readHereDoc captures everything until the delimiter appears on its own line.
func (p *Parser) readHereDoc(delim string) (string, error) {
	var builder strings.Builder
	
	for {
		line, err := p.readRawLine() // We need raw bytes to check for structural newlines
		if err == io.EOF {
			return "", &ParseError{Line: p.line, Msg: "unexpected EOF: delimiter not found"}
		}
		if err != nil {
			return "", err
		}

		// Check if this line is the closing delimiter
		// Rule: "Must have no leading whitespace"
		if strings.TrimRight(line, "\r\n\t ") == delim {
			return builder.String(), nil
		}

		builder.WriteString(line)
	}
}

// readLogicalLine increments the line counter and returns a string
func (p *Parser) readLogicalLine() (string, error) {
	line, err := p.scanner.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	p.line++
	return line, err
}

// readRawLine is for the internal content where we preserve newlines
func (p *Parser) readRawLine() (string, error) {
	line, err := p.scanner.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	p.line++
	return line, err
}