package main

import (
	"fmt"
	"go/ast"
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

	absPath, err := filepath.Abs(dir)
	if err != nil {
		log.Fatalf("Error resolving absolute path: %v", err)
	}

	fset, files, err := extractor.ParsePackage(dir)
	if err != nil {
		log.Fatalf("Error parsing package: %v", err)
	}

	err = os.MkdirAll(OUTPUT_DIRECTORY, os.ModePerm)
	if err != nil {
		log.Fatalf("Error creating output directory: %v", err)
	}

	fmt.Println("Processing files:")

	resolvedNames, err := extractor.ResolveNames(fset, files, dir)
	if err != nil {
		return
	}

	for path, astFile := range files {
		fmt.Println("File:", path)

		// Extract the filename (basename) for use of output file naming
		baseName := filepath.Base(path)
		astFileName := baseName[:len(baseName)-len(filepath.Ext(baseName))] + "_ast.json"
		outputFilePath := filepath.Join(OUTPUT_DIRECTORY, astFileName)

		err := extractor.ASTToJSON(fset, map[string]*ast.File{path: astFile}, outputFilePath, astFile.Name.Name, absPath, resolvedNames)
		if err != nil {
			log.Printf("Error processing file %s: %v", path, err)
		} else {
			fmt.Printf("AST JSON successfully written for file %s\n", path)
		}
	}
}
