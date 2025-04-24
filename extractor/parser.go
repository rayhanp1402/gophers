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

type Usage struct {
	Position Position `json:"position"`
	Path     string   `json:"path"`
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
func ASTToJSON(fset *token.FileSet, files map[string]*ast.File, outputPath string, packageName string, dir string, resolvedNames map[token.Pos]*DefinitionInfo, baseName string) error {
	var packageNodes []PackageNode

	// fmt.Printf("Resolved names: %+v\n", resolvedNames)

	// for pos, obj := range resolvedNames {
	// 	if obj == nil {
	// 		continue
	// 	}
	// 	position := fset.Position(pos)
	// 	fmt.Printf("%d: %s (in %s:%d:%d)\n",
	// 		pos,
	// 		obj.Name,           // identifier name
	// 		position.Filename,
	// 		position.Line,
	// 		position.Column,
	// 	)
	// }

	for _, file := range files {
		fileNode := FileNode{
			Filename: baseName,
		}

		// Map to track where variables, functions, etc. are used
		usageMap := make(map[string][]Usage)

		ast.Inspect(file, func(n ast.Node) bool {
			if n == nil {
				return true
			}

			pos := fset.Position(n.Pos())
			position := Position{Line: pos.Line, Column: pos.Column}

			switch x := n.(type) {
			case *ast.FuncDecl:
				if x.Recv == nil {
					// Function node
					funcNode := JSONNode{
						Type:     "Function",
						Name:     x.Name.Name,
						Params:   extractParamTypes(x.Type.Params),
						Returns:  extractParamTypes(x.Type.Results),
						Receiver: "",
						Position: position,
					}

					TrackUsages(x.Name.Name, resolvedNames, usageMap, fset)
			
					fileNode.Functions = append(fileNode.Functions, funcNode)
				} else {
					// Method node
					var receiverName string
					if starExpr, ok := x.Recv.List[0].Type.(*ast.StarExpr); ok {
						// If receiver is a pointer type, extract the underlying type (dereference)
						receiverName = extractType(starExpr.X)
					} else {
						// Handle regular (non-pointer) type receiver
						receiverName = x.Recv.List[0].Type.(*ast.Ident).Name
					}
			
					methodNode := JSONNode{
						Type:     "Method",
						Name:     x.Name.Name,
						Params:   extractParamTypes(x.Type.Params),
						Returns:  extractParamTypes(x.Type.Results),
						Receiver: receiverName,
						Position: position,
					}

					TrackUsages(x.Name.Name, resolvedNames, usageMap, fset)
			
					fileNode.Methods = append(fileNode.Methods, methodNode)
				}
			case *ast.GenDecl:
				for _, spec := range x.Specs {
					switch s := spec.(type) {
					case *ast.ValueSpec:
						for _, name := range s.Names {
							variableNode := JSONNode{
								Type:     "Variable",
								Name:     name.Name,
								Position: position,
							}

							TrackUsages(name.Name, resolvedNames, usageMap, fset)

							fileNode.Variables = append(fileNode.Variables, variableNode)
						}
					case *ast.TypeSpec:
						if t, ok := s.Type.(*ast.StructType); ok {
							structNode := JSONNode{
								Type:    "Struct",
								Name:    s.Name.Name,
								Position: position,
							}
							for _, field := range t.Fields.List {
								for _, name := range field.Names {
									structNode.Fields = append(structNode.Fields, name.Name)
								}
							}

							TrackUsages(s.Name.Name, resolvedNames, usageMap, fset)
							
							fileNode.Structs = append(fileNode.Structs, structNode)
						} else if t, ok := s.Type.(*ast.InterfaceType); ok {
							interfaceNode := JSONNode{
								Type:    "Interface",
								Name:    s.Name.Name,
								Position: position,
							}
							for _, method := range t.Methods.List {
								for _, name := range method.Names {
									interfaceNode.Methods = append(interfaceNode.Methods, name.Name)
								}
							}

							TrackUsages(s.Name.Name, resolvedNames, usageMap, fset)

							fileNode.Interfaces = append(fileNode.Interfaces, interfaceNode)
						}
					}
				}
			}

			return true
		})

		AppendUsages(fileNode.Functions, usageMap)
		AppendUsages(fileNode.Methods, usageMap)
		AppendUsages(fileNode.Variables, usageMap)
		AppendUsages(fileNode.Structs, usageMap)
		AppendUsages(fileNode.Interfaces, usageMap)

		packageNode := PackageNode{
			Name:  packageName,
			Path:  fset.Position(file.Pos()).Filename,
			File:  fileNode,
		}

		packageNodes = append(packageNodes, packageNode)
	}

	jsonData, err := json.MarshalIndent(packageNodes, "", "  ")
	if err != nil {
		return err
	}

	err = os.WriteFile(outputPath, jsonData, 0644)
	if err != nil {
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