package extractor

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Traverse all the AST nodes and print
// the info
func PrintASTToFile(fset *token.FileSet, file *ast.File, outputPath string) {
	outFile, err := os.Create(outputPath)
	if err != nil {
		log.Fatalf("Error creating file: %v", err)
	}
	defer outFile.Close()

	indent := 0
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			indent--
			return true
		}

		pos := fset.Position(n.Pos())
		end := fset.Position(n.End())
		indentStr := strings.Repeat("  ", indent)

		// Print node type and location
		fmt.Fprintf(outFile, "%s%T [%s - %s]\n", indentStr, n, pos, end)

		switch x := n.(type) {
		case *ast.Ident:
			fmt.Fprintf(outFile, "%sName: %s\n", indentStr+"  ", x.Name)

		case *ast.BasicLit:
			fmt.Fprintf(outFile, "%sLiteral: %s (%s)\n", indentStr+"  ", x.Value, x.Kind)

		case *ast.FuncDecl:
			fmt.Fprintf(outFile, "%sFunc: %s\n", indentStr+"  ", x.Name.Name)

		case *ast.GenDecl:
			fmt.Fprintf(outFile, "%sGenDecl: %s\n", indentStr+"  ", x.Tok)

		case *ast.ValueSpec:
			fmt.Fprintf(outFile, "%sValueSpec: Names = %v\n", indentStr+"  ", getIdentNames(x.Names))

		case *ast.TypeSpec:
			fmt.Fprintf(outFile, "%sTypeSpec: Name = %s\n", indentStr+"  ", x.Name.Name)

		case *ast.StructType:
			fmt.Fprintf(outFile, "%sStructType with %d field(s)\n", indentStr+"  ", len(x.Fields.List))

		case *ast.InterfaceType:
			fmt.Fprintf(outFile, "%sInterfaceType with %d method(s)\n", indentStr+"  ", len(x.Methods.List))

		case *ast.CallExpr:
			fmt.Fprintf(outFile, "%sCallExpr (Function Call)\n", indentStr+"  ")

		case *ast.AssignStmt:
			fmt.Fprintf(outFile, "%sAssignStmt: %d target(s), %d value(s)\n", indentStr+"  ", len(x.Lhs), len(x.Rhs))

		case *ast.ReturnStmt:
			fmt.Fprintf(outFile, "%sReturnStmt with %d result(s)\n", indentStr+"  ", len(x.Results))

		case *ast.IfStmt:
			fmt.Fprintf(outFile, "%sIfStmt\n", indentStr+"  ")

		case *ast.ForStmt:
			fmt.Fprintf(outFile, "%sForStmt\n", indentStr+"  ")

		case *ast.RangeStmt:
			fmt.Fprintf(outFile, "%sRangeStmt (range over %T)\n", indentStr+"  ", x.X)

		case *ast.DeclStmt:
			fmt.Fprintf(outFile, "%sDeclStmt (wraps a GenDecl)\n", indentStr+"  ")

		case *ast.ExprStmt:
			fmt.Fprintf(outFile, "%sExprStmt (expression used as a statement)\n", indentStr+"  ")

		case *ast.CompositeLit:
			fmt.Fprintf(outFile, "%sCompositeLit: Type = %T\n", indentStr+"  ", x.Type)
		}

		indent++
		return true
	})

	fmt.Printf("Full AST walked and saved to %s\n", outputPath)
}

func getIdentNames(idents []*ast.Ident) []string {
	var names []string
	for _, id := range idents {
		names = append(names, id.Name)
	}
	return names
}

// Helper function to track where the name is used
func TrackUsages(name string, resolvedNames map[token.Pos]*DefinitionInfo, usageMap map[string][]Usage, fset *token.FileSet) {
	for pos, obj := range resolvedNames {
		if obj.Name == name {
			// Get the position of the symbol's declaration
			position := fset.Position(pos)

			// Convert the token.Pos to the Position struct (line and column), and then put it inside a Usage object
			usagePosition := Usage{
				Position: Position{
					Line:   position.Line,
					Column: position.Column,
				},
				Path: position.Filename,
				Scope: obj.Scope,
			}

			// Append the usage position to the map for this name
			if usages, exists := usageMap[name]; exists {
				usageMap[name] = append(usages, usagePosition)
			} else {
				usageMap[name] = []Usage{usagePosition}
			}
		}
	}
}

func AppendUsages(nodes []JSONNode, usageMap map[string][]Usage) {
	for i := range nodes {
		node := &nodes[i]
		if usages, found := usageMap[node.Name]; found {
			node.Usages = usages
		}
	}
}

func LoadMetadata(rootDir string) ([]ProjectNode, error) {
    var allProjects []ProjectNode

    err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            return err
        }

        if d.IsDir() || filepath.Ext(path) != ".json" {
            return nil
        }

        data, err := os.ReadFile(path)
        if err != nil {
            return fmt.Errorf("reading file %s: %w", path, err)
        }

        var project ProjectNode
        if err := json.Unmarshal(data, &project); err != nil {
            return fmt.Errorf("parsing JSON %s: %w", path, err)
        }

        allProjects = append(allProjects, project)
        return nil
    })

    return allProjects, err
}

func toNodeID(path string) string {
	clean := strings.TrimSuffix(strings.ReplaceAll(path, "\\", "."), ".go")
	return strings.TrimLeft(clean, ".")
}

func extractFunctionName(scope string) string {
	// E.g., from "func main" → "main"
	parts := strings.Fields(scope)
	if len(parts) > 1 {
		return parts[1]
	}
	return scope
}