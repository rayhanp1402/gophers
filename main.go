package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"log"
	"os"
	"path/filepath"

	"github.com/rayhanp1402/gophers/extractor"
)

const PARSED_METADATA_DIRECTORY = "./parsed_metadata"

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

	err = os.MkdirAll(PARSED_METADATA_DIRECTORY, os.ModePerm)
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

		// Extract the filename (basename) for use of file metadata
		baseName := filepath.Base(path)

		// Get the relative path to preserve directory structure
		absFilePath, err := filepath.Abs(path)
		if err != nil {
			log.Printf("Error getting absolute path for %s: %v", path, err)
			continue
		}

		relPath, err := filepath.Rel(absPath, absFilePath)
		if err != nil {
			log.Printf("Error getting relative path for %s: %v", absFilePath, err)
			continue
		}

		// Change extension to .json
		jsonFileName := relPath[:len(relPath)-len(filepath.Ext(relPath))] + ".json"
		outputFilePath := filepath.Join(PARSED_METADATA_DIRECTORY, jsonFileName)

		// Ensure subdirectories are created
		err = os.MkdirAll(filepath.Dir(outputFilePath), os.ModePerm)
		if err != nil {
			log.Printf("Error creating directory for %s: %v", outputFilePath, err)
			continue
		}

		err = extractor.ASTToJSON(fset, map[string]*ast.File{path: astFile}, outputFilePath, astFile.Name.Name, absPath, resolvedNames, baseName)
		if err != nil {
			log.Printf("Error processing file %s: %v", path, err)
		} else {
			fmt.Printf("AST JSON successfully written for file %s\n", path)
		}
	}

	pkgs, err := extractor.LoadMetadata(PARSED_METADATA_DIRECTORY)
	if err != nil {
		log.Fatalf("Error parsing metadata directory: %v", err)
	}

	nodes := extractor.GenerateNodes(pkgs)
	edges := extractor.GenerateEdges(pkgs)

	graph := extractor.Graph{
		Elements: extractor.Elements{
			Nodes: nodes,
			Edges: edges,
		},
	}

	// Write the Graph to "out" directory
	outputDir := "./out"
	outputFile := filepath.Join(outputDir, "graph.json")

	err = os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	f, err := os.Create(outputFile)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(graph); err != nil {
		log.Fatalf("Failed to encode graph to JSON: %v", err)
	}

	fmt.Println("Graph written to:", outputFile)
}
