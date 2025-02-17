package extractor

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
)

const outputDir = "./out"

func ParsePackage(pkgPath string) {
	// Ensure the output directory exists
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Use filepath.Walk to traverse the directory
	err := filepath.Walk(pkgPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("Error walking through path %s: %v", path, err)
			return err
		}

		// Only parse .go files
		if !info.IsDir() && filepath.Ext(path) == ".go" {
			// Open the Go file
			file, err := os.Open(path)
			if err != nil {
				log.Printf("Failed to open file %s: %v", path, err)
				return err
			}
			defer file.Close()

			// Create a new file set for the file
			fs := token.NewFileSet()

			// Parse the Go file into an AST
			node, err := parser.ParseFile(fs, path, file, parser.AllErrors)
			if err != nil {
				log.Printf("Failed to parse file %s: %v", path, err)
				return err
			}

			// Create output file for the current file's AST
			outputFilePath := filepath.Join(outputDir, fmt.Sprintf("%s_ast.txt", filepath.Base(path)))
			outFile, err := os.Create(outputFilePath)
			if err != nil {
				log.Printf("Failed to create output file for %s: %v", path, err)
				return err
			}
			defer outFile.Close()

			// Write the AST to the output file
			fmt.Fprintf(outFile, "AST for file: %s\n", path)
			ast.Fprint(outFile, fs, node, nil)  // Write the AST to file
			fmt.Fprintln(outFile)  // Newline for better readability

			log.Printf("AST for %s has been written to %s", path, outputFilePath)
		}
		return nil
	})

	if err != nil {
		log.Fatalf("Failed to walk through directory: %v", err)
	}
}
