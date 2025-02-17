package extractor

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
)

const rootOutputDir = "./out"

func ParsePackage(pkgPath string) {
    basePkgName := filepath.Base(pkgPath)
    outputDir := filepath.Join(rootOutputDir, basePkgName)

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

			// Output plain text AST
			outputFilePath := filepath.Join(outputDir, fmt.Sprintf("%s_ast.txt", filepath.Base(path)))
			outFile, err := os.Create(outputFilePath)
			if err != nil {
				log.Printf("Failed to create output file for %s: %v", path, err)
				return err
			}
			defer outFile.Close()

			// Write the plain AST to the output file
			fmt.Fprintf(outFile, "AST for file: %s\n", path)
			ast.Fprint(outFile, fs, node, nil)  // Write the AST to file
			fmt.Fprintln(outFile)  // Newline for better readability

			log.Printf("Plain AST for %s has been written to %s", path, outputFilePath)

			// Output JSON AST (handling cyclic references)
			jsonOutputFilePath := filepath.Join(outputDir, fmt.Sprintf("%s_ast.json", filepath.Base(path)))
			jsonOutFile, err := os.Create(jsonOutputFilePath)
			if err != nil {
				log.Printf("Failed to create JSON output file for %s: %v", path, err)
				return err
			}
			defer jsonOutFile.Close()

			// Convert AST to a simplified map for JSON serialization
			astMap := convertASTToMap(node, fs)

			// Marshal the AST map into JSON and write it to the output file
			encoder := json.NewEncoder(jsonOutFile)
			encoder.SetIndent("", "  ")  // For pretty-printing
			if err := encoder.Encode(astMap); err != nil {
				log.Printf("Failed to write JSON AST to file %s: %v", path, err)
				return err
			}

			log.Printf("JSON AST for %s has been written to %s", path, jsonOutputFilePath)
		}
		return nil
	})

	if err != nil {
		log.Fatalf("Failed to walk through directory: %v", err)
	}
}

// convertASTToMap traverses the AST and converts it into a simplified structure without cyclic references
// Includes position information (line and column)
func convertASTToMap(node ast.Node, fs *token.FileSet) interface{} {
	switch n := node.(type) {
	case *ast.File:
		// Handle File node, which can have declarations
		decls := []interface{}{}
		for _, decl := range n.Decls {
			decls = append(decls, convertASTToMap(decl, fs))
		}
		return map[string]interface{}{
			"type":    "File",
			"decls":   decls,
			"package": n.Name.Name,
			"pos":     positionToMap(n.Pos(), fs),
		}
	case *ast.GenDecl:
		// Handle General Declaration (e.g., import, const, var)
		specs := []interface{}{}
		for _, spec := range n.Specs {
			specs = append(specs, convertASTToMap(spec, fs))
		}
		return map[string]interface{}{
			"type":  "GenDecl",
			"specs": specs,
			"pos":   positionToMap(n.Pos(), fs),
		}
	case *ast.FuncDecl:
		// Handle Function Declaration
		return map[string]interface{}{
			"type": "FuncDecl",
			"name": n.Name.Name,
			"body": convertASTToMap(n.Body, fs),
			"pos":  positionToMap(n.Pos(), fs),
		}
	case *ast.BlockStmt:
		// Handle Block Statement (body of a function)
		stmts := []interface{}{}
		for _, stmt := range n.List {
			stmts = append(stmts, convertASTToMap(stmt, fs))
		}
		return map[string]interface{}{
			"type":  "BlockStmt",
			"stmts": stmts,
			"pos":   positionToMap(n.Pos(), fs),
		}
	case *ast.ExprStmt:
		// Handle Expression Statement (like method calls)
		return map[string]interface{}{
			"type": "ExprStmt",
			"expr": convertASTToMap(n.X, fs),
			"pos":  positionToMap(n.Pos(), fs),
		}
	// Add more cases for other AST nodes as necessary
	default:
		// If it's an unknown or unsupported type, return as a map with the type name
		return map[string]interface{}{
			"type": fmt.Sprintf("%T", n),
			"pos":  positionToMap(n.Pos(), fs),
		}
	}
}

// positionToMap converts the token.Pos to a map with line and column information
func positionToMap(pos token.Pos, fs *token.FileSet) map[string]int {
	if pos.IsValid() {
		position := fs.Position(pos)
		return map[string]int{
			"line":   position.Line,
			"column": position.Column,
		}
	}
	return nil
}
