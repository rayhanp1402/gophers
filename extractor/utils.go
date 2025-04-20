package extractor

import (
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"os"
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