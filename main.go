package main

import (
	"fmt"
	"os"

	"github.com/rayhanp1402/gophers/extractor"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <directory>")
		os.Exit(1)
	}

	dir := os.Args[1]

	extractor.ParsePackage(dir)
}
