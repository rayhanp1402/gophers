package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/rayhanp1402/gophers/extractor"
)

const INTERMEDIATE_REPRESENTATION_DIRECTORY = "./intermediate_representation"

func main() {
	if len(os.Args) != 2 {
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

	fmt.Println("Processing files:")

	typesInfo, typesPkg, err := extractor.LoadTypesInfo(fset, files, absPath)
	if err != nil {
		log.Fatalf("Error loading types info: %v", err)
	}
	fmt.Println("Loaded types for package:", typesPkg.Name())

	err = extractor.OutputSimplifiedASTs(fset, files, absPath, INTERMEDIATE_REPRESENTATION_DIRECTORY, typesInfo)
	if err != nil {
		log.Fatalf("Error writing simplified ASTs: %v", err)
	}
	fmt.Println("Simplified ASTs written to:", INTERMEDIATE_REPRESENTATION_DIRECTORY)

	// Load intermediate_representation directory
	simplifiedASTs, err := extractor.LoadSimplifiedASTs(INTERMEDIATE_REPRESENTATION_DIRECTORY)
	if err != nil {
		log.Fatalf("Failed to load simplified ASTs: %v", err)
	}

	// Build symbol table from all files
	symbolTable := make(map[string]*extractor.ModifiedDefinitionInfo)
	for _, root := range simplifiedASTs {
		fileSymbols := extractor.CollectSymbolTable(root)
		for name, def := range fileSymbols {
			symbolTable[name] = def
		}
	}

	err = extractor.WriteSymbolTableToFile(symbolTable, "symbol_table.txt")
	if err != nil {
		log.Fatalf("Error writing symbol table: %v", err)
	}
	fmt.Println("Symbol table written to: symbol_table.txt")

	for _, root := range simplifiedASTs {
		err := extractor.SaveSimplifiedAST(root, absPath, INTERMEDIATE_REPRESENTATION_DIRECTORY)
		if err != nil {
			log.Printf("Failed to save updated AST with declaration info: %v", err)
		}
	}

	nodes, err := extractor.GenerateGraphNodes(absPath, files, symbolTable, simplifiedASTs)
	if err != nil {
		log.Fatalf("Failed to generate graph nodes: %v", err)
	}
	edges := extractor.GenerateAllEdges(simplifiedASTs, symbolTable, absPath)

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
