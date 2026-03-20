package main

import (
	"fmt"

	"exeselfcheck/selfcheck"
)

func main() {
	state, err := selfcheck.VerifyAtStartup()
	if err != nil {
		fmt.Println("selfcheck state:", state)
	}

	fmt.Println("Hello from protected app")
}
