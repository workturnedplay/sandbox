package cfgbollocks

import (
//"fmt"
	"errors"
	"strings"
)

type Entry struct {
	Key       string
	Value     string
	Delimiter string
}

var (
	errInvalidHeader   = errors.New("invalid cfgbollocks header")
	errUnsupportedVer  = errors.New("unsupported cfgbollocks version")
	errSyntax          = errors.New("syntax error")
)

func Parse(input string) ([]Entry, error) {
	// Strip UTF-8 BOM if present
	if strings.HasPrefix(input, "\uFEFF") {
		input = strings.TrimPrefix(input, "\uFEFF")
	}

	lines := splitKeepNL(input)
	if len(lines) == 0 {
		return nil, errInvalidHeader
	}

	// ---- HEADER ----
	h := lines[0]
	if strings.HasPrefix(h, "\n") || strings.HasPrefix(h, " ") || strings.HasPrefix(h, "\t") {
		return nil, errInvalidHeader
	}

	decl, delim, err := parseHeader(h)
	if err != nil {
		return nil, err
	}

	var entries []Entry
	for i := 1; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\n")
		if strings.TrimSpace(line) == "" {
			continue
		}

		key, value, usedDelim, err := parseRecord(line, decl, delim)
		if err != nil {
			return nil, err
		}

		entries = append(entries, Entry{
			Key:       key,
			Value:     value,
			Delimiter: usedDelim,
		})
	}

	return entries, nil
}

// ---------- helpers ----------

func splitKeepNL(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

/*func parseHeader(line string) (decl string, delim string, err error) {
	// cfgbollocks = DECL DELIM DECL <ws> DELIM v1 DELIM
	const key = "cfgbollocks"
	if !strings.HasPrefix(line, key) {
		return "", "", errInvalidHeader
	}

	rest := strings.TrimSpace(line[len(key):])
	if !strings.HasPrefix(rest, "=") {
		return "", "", errInvalidHeader
	}
	rest = strings.TrimSpace(rest[1:])

	// DECL DELIM DECL
	if len(rest) < 3 {
		return "", "", errInvalidHeader
	}

	d := rest[0]
	end := strings.IndexByte(rest[1:], d)
	if end < 0 {
		return "", "", errInvalidHeader
	}
	decl = rest[1 : 1+end]
	if strings.TrimSpace(decl) != decl || decl == "" {
		return "", "", errInvalidHeader
	}
	rest = strings.TrimSpace(rest[1+end+1:])

	// DELIM v1 DELIM
	if len(rest) < 2 {
		return "", "", errInvalidHeader
	}
	delim = string(rest[0])
	end = strings.Index(rest[1:], delim)
	if end < 0 {
		return "", "", errInvalidHeader
	}
	version := rest[1 : 1+end]
	if version != "v1" {
		//fmt.Println("!!!!",version,"!!!")
		return "", "", errUnsupportedVer
	}

	return decl, delim, nil
}*/
func parseHeader(line string) (decl string, delim string, err error) {
	const key = "cfgbollocks"

	if !strings.HasPrefix(line, key) {
		return "", "", errInvalidHeader
	}

	rest := strings.TrimSpace(line[len(key):])
	if !strings.HasPrefix(rest, "=") {
		return "", "", errInvalidHeader
	}
	rest = strings.TrimSpace(rest[1:])

	// Read DECL (non-whitespace, non-empty)
	i := 0
	for i < len(rest) && rest[i] != ' ' && rest[i] != '\t' {
		i++
	}
	if i == 0 {
		return "", "", errInvalidHeader
	}
	decl = rest[:i]
	rest = rest[i:]

	// Find second occurrence of DECL to extract DELIM
	idx := strings.Index(rest, decl)
	if idx < 0 {
		return "", "", errInvalidHeader
	}

	delim = rest[:idx]
	if delim == "" {
		return "", "", errInvalidHeader
	}

	rest = strings.TrimSpace(rest[idx+len(decl):])

	// Parse version: DELIM v1 DELIM
	if !strings.HasPrefix(rest, delim) {
		return "", "", errInvalidHeader
	}
	rest = rest[len(delim):]

	end := strings.Index(rest, delim)
	if end < 0 {
		return "", "", errInvalidHeader
	}

	version := rest[:end]
	if version != "v1" {
		return "", "", errUnsupportedVer
	}

	return decl, delim, nil
}

func parseRecord(line, decl, defaultDelim string) (key, value, usedDelim string, err error) {
	eq := strings.Index(line, "=")
	if eq < 0 {
		return "", "", "", errSyntax
	}

	key = strings.TrimSpace(line[:eq])
	if key == "" {
		return "", "", "", errSyntax
	}

	rest := strings.TrimSpace(line[eq+1:])

	usedDelim = defaultDelim

	// Optional inline delimiter
	if strings.HasPrefix(rest, decl) {
		rest = rest[len(decl):]
		end := strings.Index(rest, decl)
		if end < 0 {
			return "", "", "", errSyntax
		}
		usedDelim = rest[:end]
		if usedDelim == "" {
			return "", "", "", errSyntax
		}
		rest = strings.TrimSpace(rest[end+len(decl):])
	}

	if !strings.HasPrefix(rest, usedDelim) {
		return "", "", "", errSyntax
	}
	rest = rest[len(usedDelim):]

	end := strings.Index(rest, usedDelim)
	if end < 0 {
		return "", "", "", errSyntax
	}

	value = rest[:end]
	if strings.TrimSpace(rest[end+len(usedDelim):]) != "" {
		return "", "", "", errSyntax
	}

	if strings.Contains(value, usedDelim) {
		return "", "", "", errSyntax
	}

	return key, value, usedDelim, nil
}
