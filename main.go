package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/rayhanp1402/gophers/extractor"
)

const (
	IntermediateDir = "./intermediate_representation"
	OutputDir       = "./knowledge_graph"
	OutputFileName  = "graph.json"
	SymbolTableFile = "symbol_table.txt"
)

func main() {
	start := time.Now()

	// Parse command-line arguments
	debug := flag.Bool("debug", false, "Keep intermediate files and symbol table for debugging")
	flag.Usage = func() {
		fmt.Println("Usage: go run main.go [flags] <directory>")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	inputDir := flag.Arg(0)

	// Resolve absolute path
	absPath, err := filepath.Abs(inputDir)
	if err != nil {
		log.Fatalf("Failed to resolve absolute path: %v", err)
	}

	// Parse Go source files
	fset, parsedFiles, err := extractor.ParsePackage(inputDir)
	if err != nil {
		log.Fatalf("Failed to parse package: %v", err)
	}
	fmt.Println("Processing files...")

	// Load type information
	typesInfo, typesPkg, err := extractor.LoadTypesInfo(fset, parsedFiles, absPath)
	if err != nil {
		log.Fatalf("Failed to load types info: %v", err)
	}
	fmt.Println("Loaded types for package:", typesPkg.Name())

	// Output simplified ASTs
	err = extractor.OutputSimplifiedASTs(fset, parsedFiles, absPath, IntermediateDir, typesInfo)
	if err != nil {
		log.Fatalf("Failed to write simplified ASTs: %v", err)
	}
	fmt.Println("Simplified ASTs written to:", IntermediateDir)

	// Load simplified ASTs
	simplifiedASTs, err := extractor.LoadSimplifiedASTs(IntermediateDir)
	if err != nil {
		log.Fatalf("Failed to load simplified ASTs: %v", err)
	}

	// Build symbol table
	symbolTable := make(map[string]*extractor.ModifiedDefinitionInfo)
	for _, root := range simplifiedASTs {
		for name, def := range extractor.CollectSymbolTable(root) {
			symbolTable[name] = def
		}
	}

	// Optionally write symbol table
	if *debug {
		if err := extractor.WriteSymbolTableToFile(symbolTable, SymbolTableFile); err != nil {
			log.Fatalf("Failed to write symbol table: %v", err)
		}
		fmt.Println("Symbol table written to:", SymbolTableFile)
	}

	// Save updated ASTs with declaration info
	for _, root := range simplifiedASTs {
		if err := extractor.SaveSimplifiedAST(root, absPath, IntermediateDir); err != nil {
			log.Printf("Warning: failed to save updated AST: %v", err)
		}
	}

	// Generate graph data
	nodes, err := extractor.GenerateGraphNodes(absPath, parsedFiles, symbolTable, simplifiedASTs)
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

	// Write graph JSON output
	if err := os.MkdirAll(OutputDir, os.ModePerm); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}
	outputFile := filepath.Join(OutputDir, OutputFileName)

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

	// Cleanup if not in debug mode
	if !*debug {
		if err := os.RemoveAll(IntermediateDir); err != nil {
			log.Printf("Warning: failed to remove intermediate directory: %v", err)
		}
		if err := os.Remove(SymbolTableFile); err != nil && !os.IsNotExist(err) {
			log.Printf("Warning: failed to remove symbol table file: %v", err)
		}
	}

	elapsed := time.Since(start)
	fmt.Printf("Extraction completed in %s\n", elapsed)
}
