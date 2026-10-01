package main

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

// isTerminal checks whether the given file descriptor is attached to an interactive terminal window
// versus a pipe or file redirection (os.ModeCharDevice bit set).
func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// pauseExit opens the raw Windows CONIN$ console device directly.
// In Windows, entering Ctrl+Z sends an EOF to os.Stdin, causing subsequent reads on stdin to fail immediately.
// Opening CONIN$ creates a fresh handle to the console input buffer, bypassing the stdin EOF latch.
func pauseExit() {
	fmt.Print("\nPress Enter to exit...")
	if conIn, err := os.Open("CONIN$"); err == nil {
		defer conIn.Close()
		var buf [1]byte
		_, _ = conIn.Read(buf[:])
	}
}

func main() {
	interactive := isTerminal(os.Stdin)

	// Mode 1: CLI positional arguments passed.
	if len(os.Args) > 1 {
		for _, arg := range os.Args[1:] {
			decoded, err := url.QueryUnescape(arg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "urldecode: error: %v\n", err)
				fmt.Println(arg)
				continue
			}
			fmt.Println(decoded)
		}
		if interactive {
			pauseExit()
		}
		return
	}

	// Mode 2: Interactive Terminal Session.
	if interactive {
		fmt.Println("=================================================================")
		fmt.Println("                       Windows URL Decoder                       ")
		fmt.Println("=================================================================")
		fmt.Println(" Instructions:")
		fmt.Println("  1. Paste percent-encoded strings below and press Enter.")
		fmt.Println("  2. To exit: Press Ctrl+Z and then hit Enter (or Ctrl+C).")
		fmt.Println("=================================================================")
		fmt.Println()
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		if interactive {
			fmt.Print("Paste URL > ")
		}

		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			if len(trimmed) > 0 {
				decoded, unescapeErr := url.QueryUnescape(trimmed)
				if unescapeErr != nil {
					fmt.Fprintf(os.Stderr, "\n[Warning] Failed to decode: %v\n\n", unescapeErr)
				} else {
					if interactive {
						fmt.Printf("Decoded:   %s\n\n", decoded)
					} else {
						fmt.Println(decoded)
					}
				}
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintf(os.Stderr, "\n[Error] Read failure: %v\n", err)
			break
		}
	}

	if interactive {
		pauseExit()
	}
}