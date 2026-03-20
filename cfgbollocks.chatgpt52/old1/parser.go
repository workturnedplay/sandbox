package cfgbollocks

import (
	"fmt"
	"strings"
)

type Entry struct {
	Key   string
	Value string
}

type ParseOption func(*parseOptions)

type parseOptions struct {
	// reserved for future (spans, lax mode, etc.)
}

func Parse(input string, _ ...ParseOption) ([]Entry, error) {
	lines := splitLines(input)

	var entries []Entry

	const (
		stateKey = iota
		stateValue
	)

	state := stateKey

	var (
		currentKey string
		delimiter  string
		valueLines []string
	)

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		switch state {

		case stateKey:
			if isBlank(line) {
				continue
			}

			key, delim, ok := parseKeyLine(line)
			if !ok {
				return nil, parseError(i+1, "invalid key line")
			}

			currentKey = key
			delimiter = delim
			valueLines = valueLines[:0]
			state = stateValue

		case stateValue:
			if isClosingDelimiter(line, delimiter) {
				//val := strings.Join(valueLines, "\n")+"\n"
				var b strings.Builder

				for _, l := range valueLines {
					b.WriteString(l)
					b.WriteByte('\n')
				}
				var val string = b.String()

				entries = append(entries, Entry{
					Key:   currentKey,
					Value: val,
				})

				currentKey = ""
				delimiter = ""
				valueLines = valueLines[:0] // slice reuse
				state = stateKey
				continue
			}

			valueLines = append(valueLines, line)
		}
	}

	if state == stateValue {
		return nil, parseError(len(lines), "unterminated value")
	}

	return entries, nil
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	// strings.Split keeps trailing empty line behavior correct
	if strings.HasSuffix(s, "\n") {
		return strings.Split(s[:len(s)-1], "\n")
	}
	return strings.Split(s, "\n")
}

func parseKeyLine(line string) (key, delim string, ok bool) {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}

	key = strings.TrimSpace(parts[0])
	if key == "" {
		return "", "", false
	}

	delim = strings.TrimSpace(parts[1])
	if delim == "" {
		return "", "", false
	}

	return key, delim, true
}

func isClosingDelimiter(line, delim string) bool {
	if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
		return false
	}

	trimmed := strings.TrimRight(line, " \t")
	return trimmed == delim
}

func isBlank(line string) bool {
	return strings.TrimSpace(line) == ""
}

func parseError(line int, msg string) error {
	return fmt.Errorf("parse error at line %d: %s", line, msg)
}
