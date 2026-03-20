package main

import (
	"crypto/rand"
	"fmt"
	"os"
)

const markerLen = 64

func main() {
	var b [markerLen]byte
	_, err := rand.Read(b[:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "rand.Read failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Paste this into embedded_block.go:")
	fmt.Println()
	fmt.Println("Marker: [64]byte{")
	for i := 0; i < len(b); i++ {
		if i%8 == 0 {
			fmt.Print("\t")
		}
		fmt.Printf("0x%02x", b[i])
		if i != len(b)-1 {
			fmt.Print(", ")
		}
		if i%8 == 7 {
			fmt.Println()
		}
	}
	fmt.Println("},")
	fmt.println("Don't forget to also bump LayoutVersion afterwards!")
}