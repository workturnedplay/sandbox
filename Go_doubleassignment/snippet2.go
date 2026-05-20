//(The explicit else version)
package main

import "os"

func NoFileFragmentation() bool {
	return os.Getenv("OLLAMA_NO_FILE_FRAGMENTATION") == "1"
}

func MainLogic() uint {
	var numParts uint
	if NoFileFragmentation() {
		numParts = 1
	} else {
		numParts = 16
	}
	return numParts
}

func main() {
	println(MainLogic())
}