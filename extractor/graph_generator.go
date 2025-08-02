package extractor

import (
	"fmt"
	"go/ast"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type Graph struct {
	Elements Elements `json:"elements"`
}

type Elements struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

type GraphNode struct {
	Data NodeData `json:"data"`
}

type NodeData struct {
	ID         string            `json:"id"`
	Labels     []string          `json:"labels"`
	Properties map[string]string `json:"properties"`
}

type GraphEdge struct {
	Data EdgeData `json:"data"`
}

type EdgeData struct {
	ID         string            `json:"id"`
	Label      string            `json:"label"`
	Source     string            `json:"source"`
	Target     string            `json:"target"`
	Properties map[string]string `json:"properties"`
}

func GenerateGraphNodes(
	sourceRoot string,
	files map[string]*ast.File,
	symbols map[string]*ModifiedDefinitionInfo,
	simplifiedASTs map[string]*SimplifiedASTNode,
) ([]GraphNode, error) {

	nodes := []GraphNode{}
	seen := map[string]bool{}

	// Add Project node
	projectNodeID := "project:" + toNodeID(sourceRoot)
	nodes = append(nodes, GraphNode{
		Data: NodeData{
			ID:     projectNodeID,
			Labels: []string{"Project"},
			Properties: map[string]string{
				"qualifiedName": filepath.ToSlash(sourceRoot),
				"simpleName":    filepath.Base(sourceRoot),
			},
		},
	})

	// Walk the file tree to generate folder/file nodes
	err := filepath.Walk(sourceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		normalizedPath := filepath.ToSlash(path)

		if info.IsDir() && filepath.Base(path) == "intermediate_representation" {
			return filepath.SkipDir
		}

		if info.IsDir() {
			id := toNodeID(normalizedPath)
			if !seen[id] {
				nodes = append(nodes, GraphNode{
					Data: NodeData{
						ID:     id,
						Labels: []string{"Folder"},
						Properties: map[string]string{
							"qualifiedName": normalizedPath,
							"simpleName":    filepath.Base(normalizedPath),
						},
					},
				})
				seen[id] = true
			}
			return nil
		}

		if filepath.Ext(path) == ".go" {
			id := toNodeID(normalizedPath + ".go")
			if !seen[id] {
				nodes = append(nodes, GraphNode{
					Data: NodeData{
						ID:     id,
						Labels: []string{"File"},
						Properties: map[string]string{
							"qualifiedName": normalizedPath,
							"simpleName":    filepath.Base(normalizedPath),
						},
					},
				})
				seen[id] = true
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Add declaration nodes (functions, types, fields, etc.)
	for _, def := range symbols {
		// Only skip local definitions that are not param/field/var
		if def.Kind == "local" && def.Kind != "param" && def.Kind != "field" && def.Kind != "var" {
			continue
		}

		posKey := fmt.Sprintf("%s:%d:%d", def.URI, def.Line, def.Character)
		id := toNodeID(posKey)

		if seen[id] {
			continue
		}

		// Only skip primitive *types*
		if isPrimitiveType(def.Name) && (def.Kind == "type" || def.Kind == "struct" || def.Kind == "interface") {
			continue
		}

		properties := map[string]string{
			"simpleName":    def.Name,
			"qualifiedName": posKey,
			"kind":          def.Kind,
		}

		nodes = append(nodes, GraphNode{
			Data: NodeData{
				ID:         id,
				Labels:     KindToLabel(def.Kind),
				Properties: properties,
			},
		})
		seen[id] = true
	}

	// Add package nodes as Scope
	addedPackages := map[string]bool{}
	for _, root := range simplifiedASTs {
		for _, child := range root.Children {
			if child.Type == "Package" && child.Name != "" {
				pkgName := child.Name
				pkgPath := filepath.ToSlash(strings.TrimPrefix(child.Position.URI, "file://"))
				dir := filepath.Dir(pkgPath)
				qualified := fmt.Sprintf("%s/%s", dir, pkgName)

				id := toNodeID(qualified)
				if !addedPackages[id] {
					nodes = append(nodes, GraphNode{
						Data: NodeData{
							ID:     id + ".package",
							Labels: []string{"Scope"},
							Properties: map[string]string{
								"qualifiedName": qualified + ".package",
								"simpleName":    pkgName,
							},
						},
					})
					addedPackages[id] = true
				}
			}
		}
	}

	return nodes, nil
}

func KindToLabel(kind string) []string {
    switch kind {
    case "field", "var", "param":
        return []string{"Variable"}
    case "func", "method":
        return []string{"Operation", "Type"}
    case "type", "struct", "interface":
        return []string{"Type"}
    default:
        c := cases.Title(language.English)
        return []string{c.String(kind)}
    }
}

func GenerateInvokesEdges(
	simplifiedASTs map[string]*SimplifiedASTNode,
	symbols map[string]*ModifiedDefinitionInfo,
) []GraphEdge {
	var edges []GraphEdge

	// Traverse all simplified ASTs
	for _, root := range simplifiedASTs {
		var currentFuncID string

		var walk func(node *SimplifiedASTNode)
		walk = func(node *SimplifiedASTNode) {
			if node == nil {
				return
			}

			switch node.Type {
			case "Function", "Method":
				funcKey := fmt.Sprintf("%s:%d:%d", node.Position.URI, node.Position.Line, node.Position.Character)
				currentFuncID = toNodeID(funcKey)
				// Walk children (body, params, etc.) with currentFuncID
				for _, child := range node.Children {
					walk(child)
				}
				currentFuncID = "" // Clear after done

			case "Call", "MethodCall":
				if node.Name == "" || node.Position == nil || currentFuncID == "" {
					return
				}
				// Try to resolve callee name from symbol table
				for symPosKey, def := range symbols {
					if def.Name == node.Name && (def.Kind == "func" || def.Kind == "method") {
						targetID := toNodeID(symPosKey)
						AddEdge(&edges, currentFuncID, targetID, "invokes", nil)
						break // Stop after first match
					}
				}

			default:
				for _, child := range node.Children {
					walk(child)
				}
			}
		}

		walk(root)
	}

	return edges
}

func GenerateReturnsEdges(
	simplifiedASTs map[string]*SimplifiedASTNode,
	symbols map[string]*ModifiedDefinitionInfo,
) []GraphEdge {
	var edges []GraphEdge

	// Map type name to node ID
	typeNameToID := map[string]string{}
	for key, def := range symbols {
		if def.Kind == "type" || def.Kind == "struct" || def.Kind == "interface" {
			if !isPrimitiveType(def.Name) {
				typeNameToID[def.Name] = toNodeID(key)
			}
		}
	}

	for _, root := range simplifiedASTs {
		var walk func(node *SimplifiedASTNode)
		walk = func(node *SimplifiedASTNode) {
			if (node.Type == "Function" || node.Type == "Method") && node.Position != nil {
				sourceKey := fmt.Sprintf("%s:%d:%d", node.Position.URI, node.Position.Line, node.Position.Character)
				sourceID := toNodeID(sourceKey)

				for _, child := range node.Children {
					if child.Type == "Results" {
						for _, result := range child.Children {
							for _, ret := range result.Children {
								// Use declaredAt if available
								if ret.DeclaredAt != nil {
									typeName := ret.DeclaredAt.Name
									if targetID, ok := typeNameToID[typeName]; ok {
										edgeID := fmt.Sprintf("%s->%s.returns", sourceID, targetID)
										edges = append(edges, GraphEdge{
											Data: EdgeData{
												ID:     edgeID,
												Label:  "returns",
												Source: sourceID,
												Target: targetID,
												Properties: map[string]string{
													"from": node.Name,
													"to":   typeName,
												},
											},
										})
									}
								}
							}
						}
					}
				}
			}

			for _, child := range node.Children {
				walk(child)
			}
		}
		walk(root)
	}

	return edges
}

func GenerateParameterizesEdges(
	simplifiedASTs map[string]*SimplifiedASTNode,
	symbols map[string]*ModifiedDefinitionInfo,
) []GraphEdge {
	var edges []GraphEdge

	for _, root := range simplifiedASTs {
		var walk func(node *SimplifiedASTNode)
		walk = func(node *SimplifiedASTNode) {
			if node.Type == "Function" || node.Type == "Method" {
				if node.Position == nil {
					return
				}
				funcKey := fmt.Sprintf("%s:%d:%d", node.Position.URI, node.Position.Line, node.Position.Character)
				funcID := toNodeID(funcKey)

				for _, child := range node.Children {
					if child.Type == "Params" {
						for _, field := range child.Children {
							for _, ident := range field.Children {
								if ident.Type == "Ident" && ident.Position != nil {
									paramKey := fmt.Sprintf("%s:%d:%d", ident.Position.URI, ident.Position.Line, ident.Position.Character)
									if def, ok := symbols[paramKey]; ok && def.Kind == "param" {
										paramID := toNodeID(paramKey)

										edges = append(edges, GraphEdge{
											Data: EdgeData{
												ID:     fmt.Sprintf("%s->%s.parameterizes", paramID, funcID),
												Label:  "parameterizes",
												Source: paramID,
												Target: funcID,
												Properties: map[string]string{
													"name": def.Name,
												},
											},
										})
									}
								}
							}
						}
					}
				}
			}
			for _, child := range node.Children {
				walk(child)
			}
		}
		walk(root)
	}

	return edges
}

func GenerateTypeEncapsulatesVariableEdges(
	simplifiedASTs map[string]*SimplifiedASTNode,
	symbols map[string]*ModifiedDefinitionInfo,
) []GraphEdge {
	var edges []GraphEdge

	for _, astRoot := range simplifiedASTs {
		var walk func(node *SimplifiedASTNode)
		walk = func(node *SimplifiedASTNode) {
			if node.Type == "Struct" {
				structKey := fmt.Sprintf("%s:%d:5", node.Position.URI, node.Position.Line)
				structID := toNodeID(structKey)

				for _, field := range node.Children {
					if field.Type != "Field" {
						continue
					}
					for _, ident := range field.Children {
						if ident.Type != "Ident" {
							continue
						}
						fieldKey := fmt.Sprintf("%s:%d:%d", ident.Position.URI, ident.Position.Line, ident.Position.Character)
						fieldID := toNodeID(fieldKey)

						edges = append(edges, GraphEdge{
							Data: EdgeData{
								ID:     fmt.Sprintf("%s->%s.encapsulates", structID, fieldID),
								Label:  "encapsulates",
								Source: structID,
								Target: fieldID,
								Properties: map[string]string{
									"field": ident.Name,
								},
							},
						})
					}
				}
			}

			for _, child := range node.Children {
				walk(child)
			}
		}
		walk(astRoot)
	}

	return edges
}

func GenerateTypedEdges(
	symbols map[string]*ModifiedDefinitionInfo,
) []GraphEdge {
	var edges []GraphEdge

	// Build a map of all known type definitions using fully qualified names
	typeNodeMap := map[string]string{}
	for key, def := range symbols {
		if def.Kind == "struct" || def.Kind == "interface" || def.Kind == "type" {
			qualified := def.Name
			if def.PackageName != "" {
				qualified = def.PackageName + "." + def.Name
			}
			typeNodeMap[qualified] = toNodeID(key)
		}
	}

	// For every variable/field/param, check if its Type maps to a known type
	for symKey, def := range symbols {
		if def.Kind != "param" && def.Kind != "var" && def.Kind != "field" {
			continue
		}

		typeName := strings.TrimLeft(def.Type, "*[]") // clean pointer/slice prefix

		if typeID, ok := typeNodeMap[typeName]; ok {
			edges = append(edges, GraphEdge{
				Data: EdgeData{
					ID:     fmt.Sprintf("%s->%s.typed", toNodeID(symKey), typeID),
					Label:  "typed",
					Source: toNodeID(symKey),
					Target: typeID,
					Properties: map[string]string{
						"type": def.Type, // preserve original form (e.g., "*models.X")
					},
				},
			})
		}
	}

	return edges
}

func GenerateTypeEncapsulatesOperationEdges(symbols map[string]*ModifiedDefinitionInfo) []GraphEdge {
	var edges []GraphEdge

	// Map type name → node ID
	typeNameToID := make(map[string]string)
	for id, sym := range symbols {
		if sym.Kind == "struct" || sym.Kind == "interface" {
			typeNameToID[sym.Name] = id
		}
	}

	for id, sym := range symbols {
		if sym.Kind == "method" && sym.ReceiverType != "" {
			if typeID, ok := typeNameToID[sym.ReceiverType]; ok {
				edgeID := fmt.Sprintf("%s_encapsulates_%s", typeID, id)

				edges = append(edges, GraphEdge{
					Data: EdgeData{
						ID:         edgeID,
						Label:      "encapsulates",
						Source:     typeID,
						Target:     id,
						Properties: map[string]string{},
					},
				})
			}
		}
	}

	return edges
}

func GenerateScopeEnclosesTypeEdges(
	symbols map[string]*ModifiedDefinitionInfo,
) []GraphEdge {
	var edges []GraphEdge

	for _, def := range symbols {
		if def.Kind != "struct" && def.Kind != "interface" && def.Kind != "type" {
			continue
		}

		if def.URI == "" || def.PackageName == "" {
			continue
		}

		filePath := filepath.ToSlash(def.URI)
		dir := filepath.Dir(strings.TrimPrefix(filePath, "file://"))
		scopeQualified := fmt.Sprintf("%s/%s", dir, def.PackageName)
		scopeID := toNodeID(scopeQualified + ".package")

		typeID := toNodeID(fmt.Sprintf("%s:%d:%d", filePath, def.Line, def.Character))

		edges = append(edges, GraphEdge{
			Data: EdgeData{
				ID:     fmt.Sprintf("encloses:%s->%s", scopeID, typeID),
				Label:  "encloses",
				Source: scopeID,
				Target: typeID,
				Properties: map[string]string{
					"kind": "ScopeEnclosesType",
				},
			},
		})
	}

	return edges
}

func GenerateFolderContainsEdges(sourceRoot string) ([]GraphEdge, error) {
	var edges []GraphEdge

	err := filepath.Walk(sourceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == sourceRoot {
			// Skip root folder
			return nil
		}

		normalizedPath := filepath.ToSlash(path)
		parent := filepath.Dir(normalizedPath)
		parentID := toNodeID(filepath.ToSlash(parent))
		childID := ""

		if info.IsDir() {
			childID = toNodeID(normalizedPath)
		} else if filepath.Ext(path) == ".go" {
			childID = toNodeID(normalizedPath + ".go")
		} else {
			return nil // Skip non-Go files
		}

		edgeID := fmt.Sprintf("%s->%s.contains", parentID, childID)
		edges = append(edges, GraphEdge{
			Data: EdgeData{
				ID:     edgeID,
				Label:  "contains",
				Source: parentID,
				Target: childID,
				Properties: map[string]string{
					"kind": "FolderContains",
				},
			},
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	return edges, nil
}

func GenerateFileDeclaresEdges(symbols map[string]*ModifiedDefinitionInfo) []GraphEdge {
	var edges []GraphEdge

	for symKey, def := range symbols {
		// Filter by kind
		if def.Kind != "type" && def.Kind != "struct" && def.Kind != "interface" &&
			def.Kind != "func" && def.Kind != "method" &&
			def.Kind != "var" {
			continue
		}

		// Only include global vars
		if def.Kind == "var" && def.ReceiverType != "" {
			continue
		}

		// File ID: file://... (no need to add ".go")
		trimmed := strings.TrimPrefix(def.URI, "file://")
		fileID := toNodeID(trimmed + ".go")
		defID := toNodeID(symKey)

		edgeID := fmt.Sprintf("%s->%s.declares", fileID, defID)
		edges = append(edges, GraphEdge{
			Data: EdgeData{
				ID:     edgeID,
				Label:  "declares",
				Source: fileID,
				Target: defID,
				Properties: map[string]string{
					"kind": def.Kind,
					"name": def.Name,
				},
			},
		})
	}

	return edges
}

func GenerateFileDeclaresScopeEdges(simplifiedASTs map[string]*SimplifiedASTNode) []GraphEdge {
	var edges []GraphEdge

	for _, root := range simplifiedASTs {
		for _, child := range root.Children {
			if child.Type == "Package" && child.Name != "" && child.Position != nil {
				trimmed := strings.TrimPrefix(child.Position.URI, "file://")
				fileID := toNodeID(trimmed + ".go")
				scopePath := filepath.ToSlash(strings.TrimPrefix(child.Position.URI, "file://"))
				dir := filepath.Dir(scopePath)
				qualified := fmt.Sprintf("%s/%s", dir, child.Name)

				scopeID := toNodeID(qualified + ".package")

				edgeID := fmt.Sprintf("%s->%s.declares", fileID, scopeID)
				edges = append(edges, GraphEdge{
					Data: EdgeData{
						ID:     edgeID,
						Label:  "declares",
						Source: fileID,
						Target: scopeID,
						Properties: map[string]string{
							"kind": "Scope",
							"name": child.Name,
						},
					},
				})
			}
		}
	}

	return edges
}

func GenerateOperationUsesVariableEdges(
	simplifiedASTs map[string]*SimplifiedASTNode,
	symbols map[string]*ModifiedDefinitionInfo,
) []GraphEdge {
	var edges []GraphEdge

	for _, fileNode := range simplifiedASTs {
		for _, node := range fileNode.Children {
			if node.Type != "Function" && node.Type != "Method" {
				continue
			}
			if node.DeclaredAt == nil {
				continue
			}

			sourceKey := fmt.Sprintf("%s:%d:%d", node.DeclaredAt.URI, node.DeclaredAt.Line, 0)
			operationID := toNodeID(sourceKey)

			var uses []*SimplifiedASTNode
			collectUses(node, &uses)

			for _, use := range uses {
				if use.DeclaredAt == nil {
					continue
				}

				adjustedLine := use.DeclaredAt.Line - 1
				adjustedChar := use.DeclaredAt.Character - 1
				varPosKey := fmt.Sprintf("%s:%d:%d", use.DeclaredAt.URI, adjustedLine, adjustedChar)
				varID := toNodeID(varPosKey)

				edges = append(edges, GraphEdge{
					Data: EdgeData{
						ID:     operationID + "_uses_" + varID,
						Label:  "uses",
						Source: operationID,
						Target: varID,
						Properties: map[string]string{
							"line":      fmt.Sprintf("%d", use.Position.Line),
							"character": fmt.Sprintf("%d", use.Position.Character),
						},
					},
				})
			}
		}
	}

	return edges
}

func collectUses(node *SimplifiedASTNode, out *[]*SimplifiedASTNode) {
	if node.Type == "VarUse" || node.Type == "FieldUse" {
		*out = append(*out, node)
	}
	for _, child := range node.Children {
		collectUses(child, out)
	}
}

func GenerateRequiresEdges(
	simplifiedASTs map[string]*SimplifiedASTNode,
) []GraphEdge {
	var edges []GraphEdge

	// Map from package name to all file URIs that declare that package
	packageToFiles := make(map[string][]string)

	for _, fileNode := range simplifiedASTs {
		for _, child := range fileNode.Children {
			if child.Type == "Package" && fileNode.Position != nil {
				uri := strings.TrimPrefix(fileNode.Position.URI, "file://")
				packageToFiles[child.Name] = append(packageToFiles[child.Name], uri)
			}
		}
	}

	for _, fileNode := range simplifiedASTs {
		if fileNode.Position == nil {
			continue
		}
		sourceURI := strings.TrimPrefix(fileNode.Position.URI, "file://")

		var importedPkgs []string
		for _, child := range fileNode.Children {
			if child.Type == "Import" {
				importPath := strings.Trim(child.Name, `"`)
				importedPkgs = append(importedPkgs, importPath)
			}
		}

		for _, pkg := range importedPkgs {
			targetFiles := packageToFiles[path.Base(pkg)]
			for _, targetURI := range targetFiles {
				if targetURI == sourceURI {
					continue
				}

				sourceID := toNodeID(sourceURI) + ".go"
				targetID := toNodeID(targetURI) + ".go"

				edges = append(edges, GraphEdge{
					Data: EdgeData{
						ID:     sourceID + "_requires_" + targetID,
						Label:  "requires",
						Source: sourceID,
						Target: targetID,
						Properties: map[string]string{
							"imported": pkg,
						},
					},
				})
			}
		}
	}

	return edges
}

func GenerateProjectIncludesEdges(sourceRoot string) ([]GraphEdge, error) {
	projectNodeID := "project:" + toNodeID(sourceRoot)
	edges := []GraphEdge{}

	err := filepath.Walk(sourceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only include .go files and directories
		if !info.IsDir() && filepath.Ext(path) != ".go" {
			return nil
		}

		targetID := ""
		if info.IsDir() {
			targetID = strings.ReplaceAll(toNodeID(path), ".", "/")
		} else {
			targetID = strings.ReplaceAll(toNodeID(path), ".", "/") + ".go"
		}

		edgeID := projectNodeID + "_includes_" + targetID

		edges = append(edges, GraphEdge{
			Data: EdgeData{
				ID:     edgeID,
				Label:  "includes",
				Source: projectNodeID,
				Target: targetID,
				Properties: map[string]string{
					"type": "includes",
				},
			},
		})
		return nil
	})

	if err != nil {
		return nil, err
	}

	return edges, nil
}

func GenerateAllEdges(
	simplifiedASTs map[string]*SimplifiedASTNode,
	symbols map[string]*ModifiedDefinitionInfo,
	sourceRoot string,
) []GraphEdge {
	var allEdges []GraphEdge

	folderEdges, err := GenerateFolderContainsEdges(sourceRoot)
	if err == nil {
		allEdges = append(allEdges, folderEdges...)
	}

	// File declares Scope
	scopeDeclEdges := GenerateFileDeclaresScopeEdges(simplifiedASTs)
	allEdges = append(allEdges, scopeDeclEdges...)

	// File declares Variable, Type, Operation
	declaresEdges := GenerateFileDeclaresEdges(symbols)
	allEdges = append(allEdges, declaresEdges...)

	// Generate "invokes" edges
	invokesEdges := GenerateInvokesEdges(simplifiedASTs, symbols)
	allEdges = append(allEdges, invokesEdges...)

	// Generate "returns" edges
	returnsEdges := GenerateReturnsEdges(simplifiedASTs, symbols)
	allEdges = append(allEdges, returnsEdges...)

	// Generate "parameterizes" edges
	parameterizesEdges := GenerateParameterizesEdges(simplifiedASTs, symbols)
	allEdges = append(allEdges, parameterizesEdges...)

	// Generate Type "encapsulates" Variable edges
	typeEncapsulatesVariableEdges := GenerateTypeEncapsulatesVariableEdges(simplifiedASTs, symbols)
	allEdges = append(allEdges, typeEncapsulatesVariableEdges...)
	
	// Generate Type "encapsulates" Variable edges
	typeEncapsulatesOperationEdges := GenerateTypeEncapsulatesOperationEdges(symbols)
	allEdges = append(allEdges, typeEncapsulatesOperationEdges...)

	// Generate "typed" edges
	typedEdges := GenerateTypedEdges(symbols)
	allEdges = append(allEdges, typedEdges...)

	// Generate Scope "encloses" Type edges
	scopeEnclosesTypeEdges := GenerateScopeEnclosesTypeEdges(symbols)
	allEdges = append(allEdges, scopeEnclosesTypeEdges...)

	// Generate "uses" edges
	usesEdges := GenerateOperationUsesVariableEdges(simplifiedASTs, symbols)
	allEdges = append(allEdges, usesEdges...)

	// Generate "requires" edges
	requiresEdges := GenerateRequiresEdges(simplifiedASTs)
	allEdges = append(allEdges, requiresEdges...)

	// Generate Project "includes" Files/Folders
	projectRequiresFilesFoldersEdges, err := GenerateProjectIncludesEdges(sourceRoot)
	if err == nil {
		allEdges = append(allEdges, projectRequiresFilesFoldersEdges...)
	}

	return allEdges
}

func AddEdge(edges *[]GraphEdge, fromID, toID, label string, props map[string]string) {
	id := fmt.Sprintf("%s->%s:%s", fromID, toID, label)
	*edges = append(*edges, GraphEdge{
		Data: EdgeData{
			ID:         id,
			Label:      label,
			Source:     fromID,
			Target:     toID,
			Properties: props,
		},
	})
}