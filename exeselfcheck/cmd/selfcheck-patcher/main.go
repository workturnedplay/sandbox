package main

import (
	"fmt"
	"os"

	"exeselfcheck/selfcheck"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("usage: selfcheck-patcher <exe-path>")
		os.Exit(1)
	}

	err := selfcheck.PatchExecutable(os.Args[1])
	if err != nil {
		os.Exit(1)
	}
}
