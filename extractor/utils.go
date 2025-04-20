package extractor

import (
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"os"
	"strings"
)

// Traverse all the AST nodes and print the info
func WalkASTToFile(fset *token.FileSet, file *ast.File, outputPath string) {
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

		// Print node type with position
		fmt.Fprintf(outFile, "%s%T [%s - %s]\n", strings.Repeat("  ", indent), n, pos, end)

		// Print more info for specific node types
		switch x := n.(type) {
		case *ast.Ident:
			fmt.Fprintf(outFile, "%sName: %s\n", strings.Repeat("  ", indent+1), x.Name)
		case *ast.BasicLit:
			fmt.Fprintf(outFile, "%sLiteral: %s (%s)\n", strings.Repeat("  ", indent+1), x.Value, x.Kind)
		case *ast.FuncDecl:
			fmt.Fprintf(outFile, "%sFunc: %s\n", strings.Repeat("  ", indent+1), x.Name.Name)
		case *ast.GenDecl:
			fmt.Fprintf(outFile, "%sGenDecl: %s\n", strings.Repeat("  ", indent+1), x.Tok)
		}

		indent++
		return true
	})

	fmt.Printf("Full AST walked and saved to %s\n", outputPath)
}