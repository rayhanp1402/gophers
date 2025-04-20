package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/rayhanp1402/gophers/extractor"
)

const OUTPUT_DIRECTORY = "./out"

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <directory>")
		os.Exit(1)
	}

	dir := os.Args[1]

	fset, files, err := extractor.ParsePackage(dir)
	if err != nil {
		log.Fatalf("Error parsing package: %v", err)
	}

	fmt.Println("Parsed files:")
	for path, astFile := range files {
		fmt.Println("File:", path)

		// Extract the filename (basename) for use of output file naming
		baseName := filepath.Base(path)
		astFileName := baseName[:len(baseName)-len(filepath.Ext(baseName))] + "_ast.txt"
		outputFilePath := filepath.Join(OUTPUT_DIRECTORY, astFileName)

		extractor.WalkASTToFile(fset, astFile, outputFilePath)
	}
}
