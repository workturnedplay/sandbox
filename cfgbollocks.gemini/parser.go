package cfgbollocks

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"unicode"
)

type Grammar struct {
	Separator string
	// We use functions to define allowed character sets for keys and delimiters
	KeyStart  func(rune) bool
	KeyCont   func(rune) bool
	DelimCont func(rune) bool
}

type Settings struct {
	Version           string
	Mode              string
	ChompFinalNewline bool
	NormalizeNewline  string // "lf", "crlf", "cr", "none"
	Grammar           Grammar
}

type Entry struct {
	Key       string
	Value     string
	Separator string // Structural metadata: remembers if it was '~' or '='
	Delimiter string // Structural metadata: remembers if it was '###' or 'EOF'
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
	settings Settings
}

// charClasses maps specification keywords to unicode predicate functions
var charClasses = map[string]func(rune) bool{
	"letter":   unicode.IsLetter,
	"digit":    unicode.IsDigit,
	"space":    unicode.IsSpace,
	"nonspace": func(r rune) bool { return !unicode.IsSpace(r) },
	"any":      func(r rune) bool { return true },
}

func NewParser(r io.Reader) *Parser {
	return &Parser{
		scanner: bufio.NewReader(r),
		line:    1,
		settings: Settings{
			Version:           "v1",
			Mode:              "replace",
			ChompFinalNewline: false,
			NormalizeNewline:  "none",
			Grammar: Grammar{
				Separator: "~",
				KeyStart:  unicode.IsLetter,
				KeyCont:   func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' },
				DelimCont: func(r rune) bool { return !unicode.IsSpace(r) },
			},
		},
	}
}

func (p *Parser) Parse() ([]Entry, error) {
	// 1. Bootstrap
	if err := p.parseBootstrap(); err != nil {
		return nil, err
	}

	// 2. Main Loop
	for {
		// Peek for EOF or skip whitespace between records
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

		// If this was a cfgbollocks entry, update settings for the NEXT entry
		if entry.Key == "cfgbollocks" {
			if err := p.applySettings(entry.Value); err != nil {
				return nil, err
			}
		}
	}
	return p.entries, nil
}

func (p *Parser) parseBootstrap() error {
	// The first entry is ALWAYS parsed with hardcoded bootstrap rules.
	line, err := p.readRawLine()
	if err != nil {
		return &ParseError{Line: 1, Msg: "file empty"}
	}
	if !strings.HasPrefix(line, "cfgbollocks") {
		return &ParseError{Line: 1, Msg: "file must begin with 'cfgbollocks'"}
	}

	rem := strings.TrimPrefix(line, "cfgbollocks")
	idx := strings.Index(rem, "~")
	if idx == -1 {
		return &ParseError{Line: 1, Msg: "expected '~' separator after cfgbollocks"}
	}

	delim := strings.TrimSpace(rem[idx+1:])
	val, err := p.readHereDoc(delim)
	if err != nil {
		return err
	}

	if err := p.applySettings(val); err != nil {
		return err
	}

	p.entries = append(p.entries, Entry{
		Key:       "cfgbollocks",
		Value:     val,
		Separator: "~",
		Delimiter: delim,
	})
	return nil
}

func (p *Parser) parseEntry() (Entry, error) {
	line, err := p.readRawLine()
	if err != nil {
		return Entry{}, err
	}

	runes := []rune(line)
	i := 0

	// 1. Skip leading whitespace (ws*)
	for i < len(runes) && unicode.IsSpace(runes[i]) {
		i++
	}
	if i == len(runes) { // Empty line
		return p.parseEntry()
	}

	// 2. Parse Key
	if !p.settings.Grammar.KeyStart(runes[i]) {
		return Entry{}, &ParseError{Line: p.line - 1, Msg: "invalid character at start of key"}
	}
	keyStartIdx := i
	for i < len(runes) && p.settings.Grammar.KeyCont(runes[i]) {
		i++
	}
	key := string(runes[keyStartIdx:i])

	// 3. Parse Separator (ws* ~ ws*)
	for i < len(runes) && unicode.IsSpace(runes[i]) {
		i++
	}
	sep := p.settings.Grammar.Separator
	if !strings.HasPrefix(string(runes[i:]), sep) {
		return Entry{}, &ParseError{Line: p.line - 1, Msg: fmt.Sprintf("expected separator '%s'", sep)}
	}
	i += len([]rune(sep))
	for i < len(runes) && unicode.IsSpace(runes[i]) {
		i++
	}

	// 4. Parse Delimiter
	delimStartIdx := i
	for i < len(runes) && p.settings.Grammar.DelimCont(runes[i]) {
		i++
	}
	delim := string(runes[delimStartIdx:i])
	if delim == "" {
		return Entry{}, &ParseError{Line: p.line - 1, Msg: "missing value delimiter"}
	}

	// 5. Ensure nothing else on line (ws* only)
	for i < len(runes) && unicode.IsSpace(runes[i]) {
		i++
	}
	if i < len(runes) {
		return Entry{}, &ParseError{Line: p.line - 1, Msg: "content not allowed after delimiter"}
	}

	val, err := p.readHereDoc(delim)
	if err != nil {
		return Entry{}, err
	}

	return Entry{
		Key:       key,
		Value:     val,
		Separator: sep,
		Delimiter: delim,
	}, nil
}

func (p *Parser) applySettings(content string) error {
	lines := strings.Split(content, "\n")
	currentSection := ""

	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if strings.HasPrefix(l, "[") && strings.HasSuffix(l, "]") {
			currentSection = strings.ToLower(l[1 : len(l)-1])
			continue
		}

		parts := strings.SplitN(l, "=", 2)
		if len(parts) < 2 {
			parts = strings.SplitN(l, ":", 2)
			if len(parts) < 2 {
				continue
			}
		}

		k, v := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])

		switch currentSection {
		case "grammar":
			switch k {
			case "separator":
				// if strings.Contains(v, "\"") {
				// p.settings.Grammar.Separator = strings.Trim(v, "\"")
				// }
				// Change this to specifically remove outer quotes if they exist
				if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
					p.settings.Grammar.Separator = v[1 : len(v)-1]
				} else {
					p.settings.Grammar.Separator = v
				}
			case "key_start":
				p.settings.Grammar.KeyStart = parseCharClass(v)
			case "key_cont":
				p.settings.Grammar.KeyCont = parseCharClass(v)
			}
		case "newline":
			if k == "normalize" {
				p.settings.NormalizeNewline = v
			}
		case "value":
			if k == "chomp_final_newline" {
				p.settings.ChompFinalNewline = (v == "true")
			}
		default:
			if k == "format" && v != "v1" {
				return &ParseError{Line: p.line, Msg: "unsupported format version"}
			}
		}
	}
	return nil
}

func parseCharClass(expr string) func(rune) bool {
	parts := strings.Split(expr, "|")
	var predicates []func(rune) bool
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "'") && strings.HasSuffix(p, "'") && len(p) == 3 {
			literal := rune(p[1])
			predicates = append(predicates, func(r rune) bool { return r == literal })
		} else if fn, ok := charClasses[p]; ok {
			predicates = append(predicates, fn)
		}
	}
	return func(r rune) bool {
		for _, fn := range predicates {
			if fn(r) {
				return true
			}
		}
		return false
	}
}

func (p *Parser) readHereDoc(delim string) (string, error) {
	var builder strings.Builder
	var lastLine string

	for {
		line, err := p.readRawLine()
		if line != "" {
			if strings.TrimRight(line, "\r\n\t ") == delim {
				finalVal := builder.String()
				if p.settings.ChompFinalNewline && lastLine != "" {
					finalVal = strings.TrimSuffix(finalVal, "\n")
					finalVal = strings.TrimSuffix(finalVal, "\r")
				}
				return p.normalize(finalVal), nil
			}
			builder.WriteString(line)
			lastLine = line
		}

		if err == io.EOF {
			return "", &ParseError{Line: p.line, Msg: "unexpected EOF: delimiter not found"}
		}
		if err != nil {
			return "", err
		}
	}
}

func (p *Parser) normalize(s string) string {
	switch p.settings.NormalizeNewline {
	case "lf":
		s = strings.ReplaceAll(s, "\r\n", "\n")
		s = strings.ReplaceAll(s, "\r", "\n")
	case "crlf":
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
	if line != "" || err == nil {
		p.line++
	}
	return line, err
}

// Writer Logic

func isSafe(delim, value string) bool {
	lines := strings.Split(value, "\n")
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimRight(l, " \t\r"), delim) {
			return false
		}
	}
	return true
}

func FindSafeDelimiter(value string) string {
	candidate := "###"
	for !isSafe(candidate, value) {
		candidate += "#"
	}
	return candidate
}

func Write(w io.Writer, entries []Entry) error {
	tracker := NewParser(strings.NewReader(""))
	if len(entries) > 0 && entries[0].Key == "cfgbollocks" {
		_ = tracker.applySettings(entries[0].Value)
	}

	for i, e := range entries {
		// Use the entry's saved separator/delimiter if they exist
		sep := e.Separator
		if sep == "" {
			sep = tracker.settings.Grammar.Separator
		}

		delim := e.Delimiter
		if delim == "" || !isSafe(delim, e.Value) {
			delim = FindSafeDelimiter(e.Value)
		}

		fmt.Fprintf(w, "%s %s %s\n", e.Key, sep, delim)
		fmt.Fprint(w, e.Value)
		if !strings.HasSuffix(e.Value, "\n") && e.Value != "" {
			fmt.Fprint(w, "\n")
		}
		fmt.Fprintln(w, delim)

		// Update tracker for subsequent entries
		if i > 0 && e.Key == "cfgbollocks" {
			_ = tracker.applySettings(e.Value)
		}
	}
	return nil
}
