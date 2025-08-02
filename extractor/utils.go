package extractor

import (
	"fmt"
	"os"
	"strings"
)

func toNodeID(path string) string {
	clean := strings.TrimSuffix(strings.ReplaceAll(path, "\\", "."), ".go")
	return strings.TrimLeft(clean, ".")
}

func isPrimitiveType(name string) bool {
	switch name {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64",
		"complex64", "complex128",
		"byte", "rune",
		"bool", "string", "error":
		return true
	default:
		return false
	}
}

func WriteSymbolTableToFile(symbolTable map[string]*ModifiedDefinitionInfo, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create symbol table file: %w", err)
	}
	defer file.Close()

	for pos, info := range symbolTable {
		fmt.Fprintf(file, "Position: %s\n", pos)
		fmt.Fprintf(file, "  Name: %s\n", info.Name)
		fmt.Fprintf(file, "  Kind: %s\n", info.Kind)
		fmt.Fprintf(file, "  Type: %s\n", info.Type)
		fmt.Fprintf(file, "  URI: %s\n", info.URI)
		fmt.Fprintf(file, "  Line: %d, Character: %d\n", info.Line, info.Character)
		fmt.Fprintf(file, "  Receiver Type: %s\n\n", info.ReceiverType)
		fmt.Fprintf(file, "  Package Name: %s\n\n", info.PackageName)
	}

	return nil
}

func shouldIgnorePath(uri string) bool {
	return strings.Contains(uri, "/knowledge_graph/") || strings.Contains(uri, "/intermediate_representation/")
}
