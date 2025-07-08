package extractor

import (
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"strings"
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

func GenerateGraphNodes(sourceRoot string, files map[string]*ast.File, symbols map[string]*ModifiedDefinitionInfo, simplifiedASTs map[string]*SimplifiedASTNode) ([]GraphNode, error) {
	nodes := []GraphNode{}
	seen := map[string]bool{}

	err := filepath.Walk(sourceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		normalizedPath := filepath.ToSlash(path)

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

	for _, def := range symbols {
		if def.Kind == "local" {
			continue
		}

		id := toNodeID(fmt.Sprintf("%s:%d:%d", def.URI, def.Line, def.Character))
		if seen[id] {
			continue
		}
		nodes = append(nodes, GraphNode{
			Data: NodeData{
				ID:     id,
				Labels: []string{kindToLabel(def.Kind)},
				Properties: map[string]string{
					"simpleName":    def.Name,
					"qualifiedName": fmt.Sprintf("%s:%d:%d", def.URI, def.Line, def.Character),
					"kind":          def.Kind,
				},
			},
		})
		seen[id] = true
	}

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

func kindToLabel(kind string) string {
	switch kind {
	case "struct", "interface", "type":
		return "Type"
	case "func", "method":
		return "Operation"
	case "var", "field", "variable", "param":
		return "Variable"
	default:
		return "Unknown"
	}
}

func GenerateGraphEdges(
	nodes []GraphNode,
	simplifiedASTs map[string]*SimplifiedASTNode,
) []GraphEdge {
	nodeMap := make(map[string]GraphNode)
	for _, node := range nodes {
		nodeMap[node.Data.ID] = node
	}

	var edges []GraphEdge
	seen := make(map[string]bool)

	// Step 1: Generate "contains" edges
	for _, node := range nodes {
		qname := node.Data.Properties["qualifiedName"]
		parentDir := filepath.ToSlash(filepath.Dir(qname))
		childID := node.Data.ID

		if parentDir == "" || parentDir == "." {
			continue
		}
		if containsLabel(node.Data.Labels, "Scope") {
			continue
		}

		parentID := toNodeID(parentDir)
		if _, exists := nodeMap[parentID]; !exists {
			continue
		}

		edgeID := fmt.Sprintf("%s->%s", parentID, childID)
		if seen[edgeID] {
			continue
		}

		edges = append(edges, GraphEdge{
			Data: EdgeData{
				ID:     edgeID,
				Label:  "contains",
				Source: parentID,
				Target: childID,
				Properties: map[string]string{
					"type": "contains",
				},
			},
		})
		seen[edgeID] = true
	}

	// Step 2: Generate "invokes" edges
	for _, root := range simplifiedASTs {
		var currentFuncID string

		var walk func(node *SimplifiedASTNode)
		walk = func(node *SimplifiedASTNode) {
			if node == nil {
				return
			}

			if node.Type == "FuncDecl" {
				qname := fmt.Sprintf("%s:%d:%d", node.Position.URI, node.Position.Line, node.Position.Character)
				currentFuncID = toNodeID(qname)
			}

			if node.Type == "CallExpr" && currentFuncID != "" {
				for _, child := range node.Children {
					if child.Type == "SelectorExpr" && len(child.Children) == 2 {
						sel := child.Children[1]
						if sel.Type == "Ident" && sel.Name != "" {
							// Look for a GraphNode that represents the function
							for _, candidate := range nodes {
								if candidate.Data.Properties["kind"] == "func" &&
									candidate.Data.Properties["simpleName"] == sel.Name {
									targetID := candidate.Data.ID
									edgeID := fmt.Sprintf("%s->%s", currentFuncID, targetID)
									if !seen[edgeID] {
										edges = append(edges, GraphEdge{
											Data: EdgeData{
												ID:     edgeID,
												Label:  "invokes",
												Source: currentFuncID,
												Target: targetID,
												Properties: map[string]string{
													"type": "invokes",
												},
											},
										})
										seen[edgeID] = true
									}
									break // take the first match
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

func containsLabel(labels []string, target string) bool {
	for _, l := range labels {
		if l == target {
			return true
		}
	}
	return false
}

func GenerateNodes(projects []ProjectNode) []GraphNode {
	var nodes []GraphNode
	idCounter := 1

	projectSeen := make(map[string]bool)
	packageSeen := make(map[string]bool)

	for _, project := range projects {
		projectID := toNodeID(project.Name)

		// Add project node
		if !projectSeen[projectID] {
			nodes = append(nodes, GraphNode{
				Data: NodeData{
					ID:     projectID,
					Labels: []string{"Project"},
					Properties: map[string]string{
						"simpleName":    project.Name,
						"qualifiedName": project.Name,
						"kind":          "project",
					},
				},
			})
			projectSeen[projectID] = true
		}

		// Iterate packages inside project
		for _, pkg := range project.Packages {
			file := pkg.File
			baseID := toNodeID(pkg.Path)

			folderPath := filepath.ToSlash(filepath.Dir(pkg.Path))
			if folderPath == "." {
				folderPath = ""
			}

			var packageID string
			var qualifiedName string

			if folderPath == "" {
				packageID = toNodeID(pkg.Name)
				qualifiedName = pkg.Name
			} else {
				packageID = toNodeID(folderPath + "." + pkg.Name)
				qualifiedName = folderPath + "." + pkg.Name
			}

			// Add package node if not already added
			if !packageSeen[packageID] {
				nodes = append(nodes, GraphNode{
					Data: NodeData{
						ID:     packageID,
						Labels: []string{"Scope"},
						Properties: map[string]string{
							"simpleName":    pkg.Name,
							"qualifiedName": qualifiedName,
							"kind":          "package",
						},
					},
				})
				packageSeen[packageID] = true
			}

			// Add folder node
			folderID := toNodeID(folderPath)
			if folderPath != "" && !packageSeen[folderID] {
				nodes = append(nodes, GraphNode{
					Data: NodeData{
						ID:     folderID,
						Labels: []string{"Folder"},
						Properties: map[string]string{
							"simpleName":    filepath.Base(folderPath),
							"qualifiedName": folderPath,
							"kind":          "folder",
						},
					},
				})
				packageSeen[folderID] = true
			}

			// Add file node
			fileID := toNodeID(pkg.Path)
			if !packageSeen[fileID] {
				nodes = append(nodes, GraphNode{
					Data: NodeData{
						ID:     fileID,
						Labels: []string{"File"},
						Properties: map[string]string{
							"simpleName":    filepath.Base(pkg.Path),
							"qualifiedName": pkg.Path,
							"kind":          "file",
						},
					},
				})
				packageSeen[fileID] = true
			}

			// Add structs, interfaces, functions, methods, variables as before
			for _, s := range file.Structs {
				node := GraphNode{
					Data: NodeData{
						ID:     baseID + "." + s.Name,
						Labels: []string{"Type"},
						Properties: map[string]string{
							"simpleName":    s.Name,
							"qualifiedName": baseID + "." + s.Name,
							"kind":          "struct",
						},
					},
				}
				idCounter++
				nodes = append(nodes, node)

				// Add struct field nodes
				for _, fieldName := range s.Fields {
					fieldID := baseID + "." + s.Name + "." + fieldName
					nodes = append(nodes, GraphNode{
						Data: NodeData{
							ID:     fieldID,
							Labels: []string{"Variable"},
							Properties: map[string]string{
								"simpleName":    fieldName,
								"qualifiedName": fieldID,
								"kind":          "field",
								"struct":        s.Name,
							},
						},
					})
				}
			}

			for _, iface := range file.Interfaces {
				node := GraphNode{
					Data: NodeData{
						ID:     baseID + "." + iface.Name,
						Labels: []string{"Type"},
						Properties: map[string]string{
							"simpleName":    iface.Name,
							"qualifiedName": baseID + "." + iface.Name,
							"kind":          "interface",
						},
					},
				}
				idCounter++
				nodes = append(nodes, node)
			}

			for _, fn := range file.Functions {
				node := GraphNode{
					Data: NodeData{
						ID:     baseID + "." + fn.Name,
						Labels: []string{"Operation"},
						Properties: map[string]string{
							"simpleName":    fn.Name,
							"qualifiedName": baseID + "." + fn.Name,
							"kind":          "function",
						},
					},
				}
				idCounter++
				nodes = append(nodes, node)

				// Add parameter nodes
				for paramName, typeName := range fn.Params {
					paramID := baseID + "." + paramName
					nodes = append(nodes, GraphNode{
						Data: NodeData{
							ID:     paramID,
							Labels: []string{"Variable"},
							Properties: map[string]string{
								"simpleName":    paramName,
								"qualifiedName": paramID,
								"kind":          "variable",
								"type":          typeName,
								"function":      fn.Name,
							},
						},
					})
				}
			}

			for _, m := range file.Methods {
				node := GraphNode{
					Data: NodeData{
						ID:     baseID + "." + m.Name,
						Labels: []string{"Operation"},
						Properties: map[string]string{
							"simpleName":    m.Name,
							"qualifiedName": baseID + "." + m.Name,
							"kind":          "method",
						},
					},
				}
				idCounter++
				nodes = append(nodes, node)

				// Add parameter nodes
				for paramName, typeName := range m.Params {
					paramID := baseID + "." + paramName
					nodes = append(nodes, GraphNode{
						Data: NodeData{
							ID:     paramID,
							Labels: []string{"Variable"},
							Properties: map[string]string{
								"simpleName":    paramName,
								"qualifiedName": paramID,
								"kind":          "variable",
								"type":          typeName,
								"function":      m.Name,
							},
						},
					})
				}
			}

			for _, v := range file.Variables {
				node := GraphNode{
					Data: NodeData{
						ID:     baseID + "." + v.Name,
						Labels: []string{"Variable"},
						Properties: map[string]string{
							"simpleName":    v.Name,
							"qualifiedName": baseID + "." + v.Name,
							"kind":          "variable",
						},
					},
				}
				nodes = append(nodes, node)
			}
		}
	}

	return nodes
}

func GenerateEdges(projects []ProjectNode) []GraphEdge {
	var edges []GraphEdge
	counter := 1
	seen := make(map[string]struct{})

	allVariableIDs := make(map[string]struct{})
	globalVars := make(map[string]struct{})
	declToFile := make(map[string]string) // Map from identifier ID to file path

	skipGlobalScope := func(scope string) bool {
		return extractFunctionName(scope) == "global"
	}

	addEdge := func(sourceID, targetID, label string) {
		key := fmt.Sprintf("%s|%s|%s", sourceID, targetID, label)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		edges = append(edges, GraphEdge{
			Data: EdgeData{
				ID:         fmt.Sprintf("edge%d", counter),
				Label:      label,
				Source:     sourceID,
				Target:     targetID,
				Properties: map[string]string{},
			},
		})
		counter++
	}

	for _, project := range projects {
		projectID := toNodeID(project.Name)
		rootDirs := make(map[string]struct{})
		rootFiles := make(map[string]struct{})

		for _, pkg := range project.Packages {
			file := pkg.File
			baseID := toNodeID(pkg.Path)

			folderPath := filepath.ToSlash(filepath.Dir(pkg.Path))
			if folderPath == "." {
				folderPath = ""
			}

			var packageID string
			if folderPath == "" {
				packageID = toNodeID(pkg.Name)
			} else {
				packageID = toNodeID(folderPath + "." + pkg.Name)
			}

			fileID := toNodeID(pkg.Path)
			folderID := toNodeID(folderPath)

			if folderPath == "" {
				rootFiles[pkg.Path] = struct{}{}
			} else {
				rootDirs[folderPath] = struct{}{}
			}
			for dir := range rootDirs {
				addEdge(projectID, toNodeID(dir), "includes")
			}
			for file := range rootFiles {
				addEdge(projectID, toNodeID(file), "includes")
			}

			if folderPath != "" {
				parent := filepath.ToSlash(filepath.Dir(folderPath))
				if parent == "." {
					parent = ""
				}
				if parent != "" {
					addEdge(toNodeID(parent), folderID, "contains")
				}
				addEdge(folderID, fileID, "contains")
			}

			addEdge(fileID, packageID, "declares")
			for _, fn := range file.Functions {
				fnID := baseID + "." + fn.Name
				declToFile[fnID] = pkg.Path
				addEdge(fileID, fnID, "declares")
			}
			for _, m := range file.Methods {
				mID := baseID + "." + m.Name
				declToFile[mID] = pkg.Path
				addEdge(fileID, mID, "declares")
			}

			for _, v := range file.Variables {
				varID := baseID + "." + v.Name
				allVariableIDs[varID] = struct{}{}
				declToFile[varID] = pkg.Path

				if v.Position.Column == 1 {
					globalVars[varID] = struct{}{}
					addEdge(fileID, varID, "declares")
				}
			}

			for _, s := range file.Structs {
				addEdge(packageID, baseID+"."+s.Name, "encloses")
			}
			for _, i := range file.Interfaces {
				addEdge(packageID, baseID+"."+i.Name, "encloses")
			}

			for _, m := range file.Methods {
				if m.Receiver != "" {
					addEdge(baseID+"."+m.Name, baseID+"."+m.Receiver, "instantiates")
				}
			}

			for _, fn := range file.Functions {
				fnID := baseID + "." + fn.Name
				for _, retType := range fn.Returns {
					if retType != "" {
						typeID := baseID + "." + retType
						addEdge(fnID, typeID, "returns")
					}
				}
			}
			for _, m := range file.Methods {
				mID := baseID + "." + m.Name
				for _, retType := range m.Returns {
					if retType != "" {
						typeID := baseID + "." + retType
						addEdge(mID, typeID, "returns")
					}
				}
			}

			for _, iface := range file.Interfaces {
				typeID := baseID + "." + iface.Name
				for _, methodName := range iface.Methods {
					methodID := baseID + "." + methodName
					addEdge(typeID, methodID, "encapsulates")
				}
			}

			for _, s := range file.Structs {
				for _, field := range s.Fields {
					fieldID := baseID + "." + s.Name + "." + field
					structID := baseID + "." + s.Name
					addEdge(structID, fieldID, "encapsulates")
				}
			}

			structIDs := make(map[string]struct{})
			for _, s := range file.Structs {
				structIDs[baseID+"."+s.Name] = struct{}{}
			}
			for _, v := range file.Variables {
				if typ, ok := v.Params["type"]; ok {
					typeID := baseID + "." + typ
					if _, isStruct := structIDs[typeID]; isStruct {
						addEdge(baseID+"."+v.Name, typeID, "typed")
					}
				}
			}

			for _, fn := range file.Functions {
				for param := range fn.Params {
					addEdge(baseID+"."+param, baseID+"."+fn.Name, "parameterizes")
				}
			}
			for _, m := range file.Methods {
				for param := range m.Params {
					addEdge(baseID+"."+param, baseID+"."+m.Name, "parameterizes")
				}
			}

			processUsages := func(opID string, usages []Usage, targetName string) {
				for _, usage := range usages {
					if skipGlobalScope(usage.Scope) {
						continue
					}
					targetID := toNodeID(usage.Path) + "." + targetName
					if _, isVar := allVariableIDs[targetID]; isVar {
						if targetID != opID {
							addEdge(opID, targetID, "uses")

							sourceFileID := toNodeID(pkg.Path)
							if targetPath, ok := declToFile[targetID]; ok {
								targetFileID := toNodeID(targetPath)
								if sourceFileID != targetFileID {
									addEdge(sourceFileID, targetFileID, "requires")
								}
							}
						}
					}
				}
			}

			for _, fn := range file.Functions {
				fnID := baseID + "." + fn.Name
				for _, usage := range fn.Usages {
					processUsages(fnID, []Usage{usage}, usage.Scope)
				}
			}
			for _, m := range file.Methods {
				mID := baseID + "." + m.Name
				for _, usage := range m.Usages {
					processUsages(mID, []Usage{usage}, usage.Scope)
				}
			}

			for _, v := range file.Variables {
				varID := baseID + "." + v.Name
				for _, usage := range v.Usages {
					if skipGlobalScope(usage.Scope) {
						continue
					}

					userID := toNodeID(pkg.Path) + "." + extractFunctionName(usage.Scope)
					if userID == varID {
						fmt.Println("❌ Skipping self-edge")
						continue
					}

					addEdge(userID, varID, "uses")

					sourceFileID := toNodeID(pkg.Path)
					if targetPath, ok := declToFile[varID]; ok {
						targetFileID := toNodeID(targetPath)
						if sourceFileID != targetFileID {
							addEdge(sourceFileID, targetFileID, "requires")
						}
					}
				}

				if _, isGlobal := globalVars[varID]; !isGlobal && len(v.Usages) == 0 {
					declaringFunc := toNodeID(pkg.Path) + "." + extractFunctionName(v.Scope)
					if declaringFunc != varID {
						addEdge(declaringFunc, varID, "uses")
					}
				}
			}

			interfaceMethods := make(map[string]struct{})
			for _, iface := range file.Interfaces {
				for _, method := range iface.Methods {
					interfaceMethods[baseID+"."+method] = struct{}{}
				}
			}

			processInvokes := func(opID string, usages []Usage) {
				for _, usage := range usages {
					if skipGlobalScope(usage.Scope) {
						continue
					}
					callerID := toNodeID(usage.Path) + "." + extractFunctionName(usage.Scope)

					if _, isIfaceMethod := interfaceMethods[opID]; isIfaceMethod {
						continue
					}
					if callerID != opID {
						addEdge(callerID, opID, "invokes")

						sourceFileID := toNodeID(usage.Path)
						if targetPath, ok := declToFile[opID]; ok {
							targetFileID := toNodeID(targetPath)
							if sourceFileID != targetFileID {
								addEdge(sourceFileID, targetFileID, "requires")
							}
						}
					}
				}
			}

			for _, fn := range file.Functions {
				fnID := baseID + "." + fn.Name
				processInvokes(fnID, fn.Usages)
			}
			for _, m := range file.Methods {
				mID := baseID + "." + m.Name
				processInvokes(mID, m.Usages)
			}
		}
	}

	return edges
}