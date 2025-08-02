package extractor_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/rayhanp1402/gophers/extractor"
)

const (
	testInputDir      = "../testdata/go-backend"
	testIntermediate  = "../testdata/outputs/intermediate_representation"
	expectedGraphPath = "../testdata/outputs/knowledge_graph/graph.json"
	tmpGraphPath      = "tmp_graph.json"
)

func TestGraphBuilderAgainstExpectedOutput(t *testing.T) {
	absPath, err := filepath.Abs(testInputDir)
	if err != nil {
		t.Fatalf("Failed to get absolute path: %v", err)
	}

	_, parsedFiles, err := extractor.ParsePackage(testInputDir)
	if err != nil {
		t.Fatalf("Failed to parse package: %v", err)
	}

	simplifiedASTs, err := extractor.LoadSimplifiedASTs(testIntermediate)
	if err != nil {
		t.Fatalf("Failed to load simplified ASTs: %v", err)
	}

	symbolTable := make(map[string]*extractor.ModifiedDefinitionInfo)
	for _, root := range simplifiedASTs {
		for k, v := range extractor.CollectSymbolTable(root) {
			symbolTable[k] = v
		}
	}

	nodes, err := extractor.GenerateGraphNodes(absPath, parsedFiles, symbolTable, simplifiedASTs)
	if err != nil {
		t.Fatalf("Failed to generate graph nodes: %v", err)
	}
	edges := extractor.GenerateAllEdges(simplifiedASTs, symbolTable, absPath)

	// Sort nodes and edges by ID to ensure deterministic comparison
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Data.ID < nodes[j].Data.ID
	})
	sort.Slice(edges, func(i, j int) bool {
		return edges[i].Data.ID < edges[j].Data.ID
	})

	actual := extractor.Graph{
		Elements: extractor.Elements{
			Nodes: nodes,
			Edges: edges,
		},
	}

	f, err := os.Open(expectedGraphPath)
	if err != nil {
		t.Fatalf("Failed to open expected graph: %v", err)
	}
	defer f.Close()

	var expected extractor.Graph
	if err := json.NewDecoder(f).Decode(&expected); err != nil {
		t.Fatalf("Failed to decode expected graph: %v", err)
	}

	// Sort expected to match deterministic order
	sort.Slice(expected.Elements.Nodes, func(i, j int) bool {
		return expected.Elements.Nodes[i].Data.ID < expected.Elements.Nodes[j].Data.ID
	})
	sort.Slice(expected.Elements.Edges, func(i, j int) bool {
		return expected.Elements.Edges[i].Data.ID < expected.Elements.Edges[j].Data.ID
	})

	actualJSON, _ := json.MarshalIndent(actual, "", "  ")
	expectedJSON, _ := json.MarshalIndent(expected, "", "  ")

	// Output actual JSON to tmp_graph.json
	if err := os.WriteFile(tmpGraphPath, actualJSON, 0644); err != nil {
		t.Errorf("Failed to write actual graph to %s: %v", tmpGraphPath, err)
	}

	if string(actualJSON) != string(expectedJSON) {
		t.Errorf("Graph output does not match expected JSON.\nRun `diff` between actual and expected JSON to see changes.")
	}
}
