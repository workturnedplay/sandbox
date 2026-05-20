//(The clean "double assignment" version)
package main

import "os"

// Simulating the runtime envconfig function
func NoFileFragmentation() bool {
	return os.Getenv("OLLAMA_NO_FILE_FRAGMENTATION") == "1"
}

func MainLogic() uint {
	numParts := uint(16)
	if NoFileFragmentation() {
		numParts = 1
	}
	return numParts
}

func main() {
	println(MainLogic())
}