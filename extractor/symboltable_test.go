package extractor_test

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rayhanp1402/gophers/extractor"
)

func TestCollectSymbolTableMultipleASTs(t *testing.T) {
	testdataPath := filepath.Join("..", "testdata", "outputs", "intermediate_representation")

	asts, err := extractor.LoadSimplifiedASTs(testdataPath)
	if err != nil {
		t.Fatalf("Failed to load simplified ASTs: %v", err)
	}

	symbols := make(map[string]*extractor.ModifiedDefinitionInfo)
	for _, root := range asts {
		for name, def := range extractor.CollectSymbolTable(root) {
			symbols[name] = def
		}
	}

	if len(symbols) == 0 {
		t.Fatal("Expected symbols, got none")
	}

	// Read expected symbol keys from symbol_table.txt
	expectedPath := filepath.Join("..", "testdata", "outputs", "symbol_table.txt")
	file, err := os.Open(expectedPath)
	if err != nil {
		t.Fatalf("Failed to open symbol_table.txt: %v", err)
	}
	defer file.Close()

	expectedKeys := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Position: ") {
			key := strings.TrimPrefix(line, "Position: ")
			expectedKeys[key] = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("Failed to read symbol_table.txt: %v", err)
	}

	// Compare expected keys with actual
	for key := range expectedKeys {
		if _, ok := symbols[key]; !ok {
			t.Errorf("Expected symbol key %q not found in actual output", key)
		}
	}

	for key := range symbols {
		if !expectedKeys[key] {
			t.Errorf("Unexpected symbol key %q found in actual output", key)
		}
	}
}