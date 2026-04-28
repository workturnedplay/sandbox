package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	//"strings"
)

func main() {
	f, err := os.Create("loadmeinfirefox.txt")
	if err != nil {
		log.Fatalf("create: %v", err)
	}
	defer f.Close()

	w := bufio.NewWriterSize(f, 1<<20) // 1MB write buffer
	//line := strings.Repeat("A", 99)
	for i := range 900_000 {
		fmt.Fprintf(w, "line %07d\n", i+1)//, line)
		// if i%50_000 == 0 {
			// log.Printf("wrote %d lines", i)
		// }
	}
	if err := w.Flush(); err != nil {
		log.Fatalf("flush: %v", err)
	}
	log.Println("done")
}