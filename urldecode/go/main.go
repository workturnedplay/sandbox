package main

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

// processStream reads input line-by-line from r and writes URL-decoded strings to w.
// Uses bufio.Reader instead of bufio.Scanner to handle lines exceeding standard token buffer limits (64KB)
// without memory exhaustion or truncation.
func processStream(r io.Reader, w io.Writer) error {
	reader := bufio.NewReader(r)
	writer := bufio.NewWriter(w)
	defer writer.Flush()

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			// Strip trailing carriage returns and newlines while preserving inner white space.
			trimmed := strings.TrimRight(line, "\r\n")

			// QueryUnescape decodes %XX sequences and converts '+' to space (standard URL query behavior).
			decoded, unescapeErr := url.QueryUnescape(trimmed)
			if unescapeErr != nil {
				// Defense-in-depth: Log non-fatal decoding errors (e.g., malformed % escape) to stderr,
				// output raw string to preserve pipeline stream, and continue.
				fmt.Fprintf(os.Stderr, "urldecode: warning: failed to decode input: %v\n", unescapeErr)
				if _, writeErr := writer.WriteString(trimmed + "\n"); writeErr != nil {
					return fmt.Errorf("write error: %w", writeErr)
				}
			} else {
				if _, writeErr := writer.WriteString(decoded + "\n"); writeErr != nil {
					return fmt.Errorf("write error: %w", writeErr)
				}
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read error: %w", err)
		}
	}

	return nil
}

func main() {
	// Mode 1: Process positional command-line arguments if provided.
	if len(os.Args) > 1 {
		writer := bufio.NewWriter(os.Stdout)
		defer writer.Flush()

		for _, arg := range os.Args[1:] {
			decoded, err := url.QueryUnescape(arg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "urldecode: warning: failed to decode argument %q: %v\n", arg, err)
				writer.WriteString(arg + "\n")
				continue
			}
			writer.WriteString(decoded + "\n")
		}
		return
	}

	// Mode 2: Process stdin (interactive paste or piped input).
	if err := processStream(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "urldecode: error: %v\n", err)
		os.Exit(1)
	}
}