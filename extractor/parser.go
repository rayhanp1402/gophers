package extractor

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
)

type JSONNode struct {
	Type      string   `json:"type"`
	Name      string   `json:"name"`
	Params  map[string]string `json:"params,omitempty"`
	Returns map[string]string `json:"returns,omitempty"`
	Receiver  string   `json:"receiver,omitempty"`
	Fields    []string `json:"fields,omitempty"`
	Methods   []string `json:"methods,omitempty"`
	Variables []string `json:"variables,omitempty"`
	Position  Position `json:"position"`
	Usages    []Usage `json:"usages,omitempty"`
}

type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

type FileNode struct {
	Filename string     `json:"filename"`
	Functions []JSONNode `json:"functions"`
	Methods   []JSONNode `json:"methods"`
	Variables []JSONNode `json:"variables"`
	Structs   []JSONNode `json:"structs"`
	Interfaces []JSONNode `json:"interfaces"`
}

type PackageNode struct {
	Name    string      `json:"name"`
	Path    string      `json:"path"`
	File   	FileNode  	`json:"file"`
}

type ProjectNode struct {
	Name     string        `json:"name"`
	Packages []PackageNode `json:"packages"`
}

type Usage struct {
	Position Position `json:"position"`
	Path     string   `json:"path"`
	Scope  	 string	  `json:"scope,omitempty"`
}

// Parses a whole package (only the .go files) into a FileSet
// dir is relative to this (gophers) package
func ParsePackage(dir string) (*token.FileSet, map[string]*ast.File, error) {
    fset := token.NewFileSet()

    files := make(map[string]*ast.File)

    err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
        if filepath.Ext(path) == ".go" {
            file, err := os.Open(path)
            if err != nil {
                return err
            }
            defer file.Close()

            astFile, err := parser.ParseFile(fset, path, file, parser.AllErrors)
            if err != nil {
                return err
            }
    
            files[path] = astFile
        }

        return nil
    })

    if err != nil {
        return nil, nil, err
    }

    return fset, files, nil
}

// Traverse and extract relevant data into the custom JSON format
func ASTToJSON(
	fset *token.FileSet,
	files map[string]*ast.File,
	outputPath string,
	packageName string,
	dir string,
	resolvedNames map[token.Pos]*DefinitionInfo,
	baseName string,
) error {
	projectNode := ProjectNode{
		Name: filepath.Base(dir),
	}

	for _, file := range files {
		fileNode := FileNode{
			Filename: baseName,
		}

		usageMap := make(map[string][]Usage)
		localVars := map[string][]JSONNode{}

		ast.Inspect(file, func(n ast.Node) bool {
			if n == nil {
				return true
			}

			pos := fset.Position(n.Pos())
			position := Position{Line: pos.Line, Column: pos.Column}

			switch x := n.(type) {
			case *ast.FuncDecl:
				funcName := x.Name.Name
				scope := "func " + funcName

				node := JSONNode{
					Type:     "Function",
					Name:     funcName,
					Params:   extractParamTypes(x.Type.Params),
					Returns:  extractParamTypes(x.Type.Results),
					Receiver: "",
					Position: position,
				}

				if x.Recv != nil {
					// It's a method
					node.Type = "Method"
					if starExpr, ok := x.Recv.List[0].Type.(*ast.StarExpr); ok {
						node.Receiver = extractType(starExpr.X)
					} else if ident, ok := x.Recv.List[0].Type.(*ast.Ident); ok {
						node.Receiver = ident.Name
					}
					fileNode.Methods = append(fileNode.Methods, node)
				} else {
					// It's a plain function
					fileNode.Functions = append(fileNode.Functions, node)
				}

				TrackUsages(funcName, resolvedNames, usageMap, fset)

				// Extract local variables from the function body
				ast.Inspect(x.Body, func(bn ast.Node) bool {
					switch stmt := bn.(type) {
					case *ast.AssignStmt:
						if stmt.Tok == token.DEFINE {
							for _, lhs := range stmt.Lhs {
								if ident, ok := lhs.(*ast.Ident); ok {
									declPos := fset.Position(ident.Pos())
									localVar := JSONNode{
										Type:     "Variable",
										Name:     ident.Name,
										Position: Position{Line: declPos.Line, Column: declPos.Column},
									}
									localVars[scope] = append(localVars[scope], localVar)
								}
							}
						}
					case *ast.DeclStmt:
						if gen, ok := stmt.Decl.(*ast.GenDecl); ok {
							for _, spec := range gen.Specs {
								if vs, ok := spec.(*ast.ValueSpec); ok {
									for _, ident := range vs.Names {
										declPos := fset.Position(ident.Pos())
										localVar := JSONNode{
											Type:     "Variable",
											Name:     ident.Name,
											Position: Position{Line: declPos.Line, Column: declPos.Column},
										}
										localVars[scope] = append(localVars[scope], localVar)
									}
								}
							}
						}
					}
					return true
				})

			case *ast.GenDecl:
				for _, spec := range x.Specs {
					switch s := spec.(type) {
					case *ast.ValueSpec:
						for _, name := range s.Names {
							varNode := JSONNode{
								Type:     "Variable",
								Name:     name.Name,
								Position: position,
							}
							fileNode.Variables = append(fileNode.Variables, varNode)
							TrackUsages(name.Name, resolvedNames, usageMap, fset)
						}
					case *ast.TypeSpec:
						switch t := s.Type.(type) {
						case *ast.StructType:
							structNode := JSONNode{
								Type:     "Struct",
								Name:     s.Name.Name,
								Position: position,
							}
							for _, field := range t.Fields.List {
								for _, name := range field.Names {
									structNode.Fields = append(structNode.Fields, name.Name)
								}
							}
							fileNode.Structs = append(fileNode.Structs, structNode)
							TrackUsages(s.Name.Name, resolvedNames, usageMap, fset)

						case *ast.InterfaceType:
							ifaceNode := JSONNode{
								Type:     "Interface",
								Name:     s.Name.Name,
								Position: position,
							}
							for _, method := range t.Methods.List {
								for _, name := range method.Names {
									ifaceNode.Methods = append(ifaceNode.Methods, name.Name)
								}
							}
							fileNode.Interfaces = append(fileNode.Interfaces, ifaceNode)
							TrackUsages(s.Name.Name, resolvedNames, usageMap, fset)
						}
					}
				}
			}
			return true
		})

		// Append collected local variables and their usage info
		for scope, vars := range localVars {
			for _, v := range vars {
				fileNode.Variables = append(fileNode.Variables, v)
				TrackUsages(v.Name, resolvedNames, usageMap, fset, scope)
			}
		}

		AppendUsages(fileNode.Functions, usageMap)
		AppendUsages(fileNode.Methods, usageMap)
		AppendUsages(fileNode.Variables, usageMap)
		AppendUsages(fileNode.Structs, usageMap)
		AppendUsages(fileNode.Interfaces, usageMap)

		packageNode := PackageNode{
			Name: packageName,
			Path: fset.Position(file.Pos()).Filename,
			File: fileNode,
		}
		projectNode.Packages = append(projectNode.Packages, packageNode)
	}

	jsonData, err := json.MarshalIndent(projectNode, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(outputPath, jsonData, 0644); err != nil {
		return err
	}

	fmt.Printf("JSON successfully written to %s\n", outputPath)
	return nil
}

// Extracts parameters from a parameter list
func extractParamTypes(fields *ast.FieldList) map[string]string {
	paramMap := make(map[string]string)
	if fields != nil {
		for i, field := range fields.List {
			typeStr := extractType(field.Type)
			// If the parameter has names (e.g., func(x int)), use them
			if len(field.Names) > 0 {
				for _, name := range field.Names {
					paramMap[name.Name] = typeStr
				}
			} else {
				// Anonymous param (e.g., func(int)) – give it a generated name
				paramMap[fmt.Sprintf("param%d", i)] = typeStr
			}
		}
	}
	return paramMap
}

func extractType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.ArrayType:
		return "[]" + extractType(t.Elt)
	case *ast.StarExpr:
		return "*" + extractType(t.X)
	case *ast.SelectorExpr:
		// For imported types like `pkg.Type`
		return extractType(t.X) + "." + t.Sel.Name
	case *ast.MapType:
		return "map[" + extractType(t.Key) + "]" + extractType(t.Value)
	default:
		return fmt.Sprintf("%T", expr) // fallback
	}
}