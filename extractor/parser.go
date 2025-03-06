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
			ast.Fprint(outFile, fs, node, nil)
			fmt.Fprintln(outFile)

			log.Printf("Plain AST for %s has been written to %s", path, outputFilePath)

			// Output JSON AST
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
			encoder.SetIndent("", "  ")
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
    if node == nil {
        return nil
    }

    switch n := node.(type) {
    case *ast.File:
        decls := []interface{}{}
        for _, decl := range n.Decls {
            decls = append(decls, convertASTToMap(decl, fs))
        }
        return map[string]interface{}{
            "type":    "File",
            "package": n.Name.Name,
            "decls":   decls,
            "pos":     positionToMap(n.Pos(), fs),
        }

    // Declarations
    case *ast.GenDecl:
        specs := []interface{}{}
        for _, spec := range n.Specs {
            specs = append(specs, convertASTToMap(spec, fs))
        }
        return map[string]interface{}{
            "type":  "GenDecl",
            "tok":   n.Tok.String(),
            "specs": specs,
            "pos":   positionToMap(n.Pos(), fs),
        }
    case *ast.FuncDecl:
        return map[string]interface{}{
            "type": "FuncDecl",
            "name": n.Name.Name,
            "body": convertASTToMap(n.Body, fs),
            "pos":  positionToMap(n.Pos(), fs),
        }

    // Statements
    case *ast.BlockStmt:
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
        return map[string]interface{}{
            "type": "ExprStmt",
            "expr": convertASTToMap(n.X, fs),
            "pos":  positionToMap(n.Pos(), fs),
        }
    case *ast.AssignStmt:
        lhs := []interface{}{}
        for _, expr := range n.Lhs {
            lhs = append(lhs, convertASTToMap(expr, fs))
        }
        rhs := []interface{}{}
        for _, expr := range n.Rhs {
            rhs = append(rhs, convertASTToMap(expr, fs))
        }
        return map[string]interface{}{
            "type": "AssignStmt",
            "lhs":  lhs,
            "rhs":  rhs,
            "tok":  n.Tok.String(),
            "pos":  positionToMap(n.Pos(), fs),
        }
    case *ast.ReturnStmt:
        results := []interface{}{}
        for _, result := range n.Results {
            results = append(results, convertASTToMap(result, fs))
        }
        return map[string]interface{}{
            "type":    "ReturnStmt",
            "results": results,
            "pos":     positionToMap(n.Pos(), fs),
        }

    // Expressions
    case *ast.CallExpr:
        args := []interface{}{}
        for _, arg := range n.Args {
            args = append(args, convertASTToMap(arg, fs))
        }
        return map[string]interface{}{
            "type":    "CallExpr",
            "fun":     convertASTToMap(n.Fun, fs),
            "args":    args,
            "pos":     positionToMap(n.Pos(), fs),
        }
    case *ast.Ident:
        return map[string]interface{}{
            "type": "Ident",
            "name": n.Name,
            "pos":  positionToMap(n.Pos(), fs),
        }
    case *ast.BasicLit:
        return map[string]interface{}{
            "type":  "BasicLit",
            "value": n.Value,
            "kind":  n.Kind.String(),
            "pos":   positionToMap(n.Pos(), fs),
        }
    case *ast.BinaryExpr:
        return map[string]interface{}{
            "type": "BinaryExpr",
            "x":    convertASTToMap(n.X, fs),
            "op":   n.Op.String(),
            "y":    convertASTToMap(n.Y, fs),
            "pos":  positionToMap(n.Pos(), fs),
        }
    case *ast.UnaryExpr:
        return map[string]interface{}{
            "type": "UnaryExpr",
            "op":   n.Op.String(),
            "x":    convertASTToMap(n.X, fs),
            "pos":  positionToMap(n.Pos(), fs),
        }

    // Control Flow
    case *ast.IfStmt:
        return map[string]interface{}{
            "type": "IfStmt",
            "cond": convertASTToMap(n.Cond, fs),
            "body": convertASTToMap(n.Body, fs),
            "pos":  positionToMap(n.Pos(), fs),
        }
    case *ast.ForStmt:
        return map[string]interface{}{
            "type": "ForStmt",
            "init": convertASTToMap(n.Init, fs),
            "cond": convertASTToMap(n.Cond, fs),
            "post": convertASTToMap(n.Post, fs),
            "body": convertASTToMap(n.Body, fs),
            "pos":  positionToMap(n.Pos(), fs),
        }
    case *ast.RangeStmt:
        return map[string]interface{}{
            "type":   "RangeStmt",
            "key":    convertASTToMap(n.Key, fs),
            "value":  convertASTToMap(n.Value, fs),
            "tok":    n.Tok.String(),
            "body":   convertASTToMap(n.Body, fs),
            "pos":    positionToMap(n.Pos(), fs),
        }
    case *ast.SwitchStmt:
        return map[string]interface{}{
            "type":  "SwitchStmt",
            "tag":   convertASTToMap(n.Tag, fs),
            "body":  convertASTToMap(n.Body, fs),
            "pos":   positionToMap(n.Pos(), fs),
        }
    case *ast.CaseClause:
		list := []interface{}{}
		for _, expr := range n.List {
			list = append(list, convertASTToMap(expr, fs))
		}
	
		body := []interface{}{}
		for _, stmt := range n.Body {
			body = append(body, convertASTToMap(stmt, fs))
		}
	
		return map[string]interface{}{
			"type": "CaseClause",
			"list": list,
			"body": body,
			"pos":  positionToMap(n.Pos(), fs),
		}

    // Type-related
    case *ast.TypeSpec:
        return map[string]interface{}{
            "type": "TypeSpec",
            "name": n.Name.Name,
            "pos":  positionToMap(n.Pos(), fs),
        }
    case *ast.StructType:
        fields := []interface{}{}
        for _, field := range n.Fields.List {
            fields = append(fields, convertASTToMap(field, fs))
        }
        return map[string]interface{}{
            "type":   "StructType",
            "fields": fields,
            "pos":    positionToMap(n.Pos(), fs),
        }
    case *ast.InterfaceType:
        methods := []interface{}{}
        for _, method := range n.Methods.List {
            methods = append(methods, convertASTToMap(method, fs))
        }
        return map[string]interface{}{
            "type":    "InterfaceType",
            "methods": methods,
            "pos":     positionToMap(n.Pos(), fs),
        }

    // Default fallback for unhandled nodes
    default:
        return map[string]interface{}{
            "type": fmt.Sprintf("%T", n),
            "pos":  positionToMap(n.Pos(), fs),
        }
    }
}

// Converts the token.Pos to a map with line and column information
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
