package cfgbollocks

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

type Settings struct {
	Version           string
	Mode              string // e.g., "replace"
	ChompFinalNewline bool
	NormalizeNewline  string // "lf", "crlf", "cr", "none"
}

type Entry struct {
	Key   string
	Value string
}

type ParseError struct {
	Line int
	Msg  string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
}

type Parser struct {
	scanner  *bufio.Reader
	line     int
	entries  []Entry
	settings Settings // Current active rules
}

func NewParser(r io.Reader) *Parser {
	return &Parser{
		scanner: bufio.NewReader(r),
		line:    1,
		settings: Settings{
			// Default "hardcoded" bootstrap settings
			Version:           "v1",
			Mode:              "replace",
			ChompFinalNewline: false,
			NormalizeNewline:  "none",
		},
	}
}

func (p *Parser) Parse() ([]Entry, error) {
	if err := p.parseBootstrap(); err != nil {
		return nil, err
	}

	for {
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

func (p *Parser) parseBootstrap() error {
	const keyName = "cfgbollocks"
	line, err := p.readRawLine()
	if err != nil {
		return &ParseError{Line: 1, Msg: "file empty"}
	}

	// Rule: No whitespace before first entry
	if !strings.HasPrefix(line, keyName) {
		return &ParseError{Line: 1, Msg: "must start with cfgbollocks"}
	}

	rem := strings.TrimPrefix(line, keyName)
	// Find separator ~
	idx := strings.Index(rem, "~")
	if idx == -1 {
		return &ParseError{Line: 1, Msg: "missing ~"}
	}

	delim := strings.TrimSpace(rem[idx+1:])
	if delim == "" {
		return &ParseError{Line: 1, Msg: "missing delimiter"}
	}

	val, err := p.readHereDoc(delim)
	if err != nil {
		return err
	}

	// Evaluate the bootstrap value to set initial settings
	if err := p.applySettings(val); err != nil {
		return err
	}

	p.entries = append(p.entries, Entry{Key: keyName, Value: val})
	return nil
}

func (p *Parser) parseEntry() (Entry, error) {
	line, err := p.readRawLine()
	if err != nil {
		return Entry{}, err
	}

	// Clean line for key extraction
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return p.parseEntry()
	}

	idx := strings.Index(trimmed, "~")
	if idx == -1 {
		return Entry{}, &ParseError{Line: p.line - 1, Msg: "missing ~"}
	}

	key := strings.TrimSpace(trimmed[:idx])
	delim := strings.TrimSpace(trimmed[idx+1:])

	val, err := p.readHereDoc(delim)
	if err != nil {
		return Entry{}, err
	}

	return Entry{Key: key, Value: val}, nil
}

// applySettings parses the content of a cfgbollocks value.
// It uses a simple key=value logic for the knobs.
func (p *Parser) applySettings(content string) error {
	lines := strings.Split(content, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "[") { // Skip section headers for now
			continue
		}

		parts := strings.SplitN(l, "=", 2)
		if len(parts) < 2 {
			continue 
		}

		k, v := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		switch k {
		case "format":
			if v != "v1" {
				return &ParseError{Line: p.line, Msg: "unsupported format version: " + v}
			}
			p.settings.Version = v
		case "mode":
			p.settings.Mode = v
		case "chomp_final_newline":
			p.settings.ChompFinalNewline = (v == "true")
		case "normalize":
			p.settings.NormalizeNewline = v
		}
	}
	return nil
}

func (p *Parser) readHereDoc(delim string) (string, error) {
	var builder strings.Builder
	var lastLine string

	for {
		line, err := p.readRawLine()
		if err == io.EOF {
			return "", &ParseError{Line: p.line, Msg: "delimiter not found"}
		}
		if err != nil {
			return "", err
		}

		// Check for closing delimiter (no leading whitespace)
		if strings.TrimRight(line, "\r\n\t ") == delim {
			finalVal := builder.String()
			
			// APPLY SEMANTICS: Chomp
			if p.settings.ChompFinalNewline && lastLine != "" {
				// Remove the newline that was added at the end of the previous iteration
				finalVal = strings.TrimSuffix(finalVal, "\n")
				finalVal = strings.TrimSuffix(finalVal, "\r")
			}

			// APPLY SEMANTICS: Normalization
			return p.normalize(finalVal), nil
		}

		builder.WriteString(line)
		lastLine = line
	}
}

func (p *Parser) normalize(s string) string {
	switch p.settings.NormalizeNewline {
	case "lf":
		s = strings.ReplaceAll(s, "\r\n", "\n")
		s = strings.ReplaceAll(s, "\r", "\n")
	case "crlf":
		// Standardize to LF first, then to CRLF
		s = strings.ReplaceAll(s, "\r\n", "\n")
		s = strings.ReplaceAll(s, "\r", "\n")
		s = strings.ReplaceAll(s, "\n", "\r\n")
	}
	return s
}

func (p *Parser) readRawLine() (string, error) {
	line, err := p.scanner.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	if line != "" {
		p.line++
	}
	return line, err
}