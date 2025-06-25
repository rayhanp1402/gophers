package extractor

import (
	"fmt"
	"path/filepath"
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
				addEdge(fileID, fnID, "declares")
			}
			for _, m := range file.Methods {
				mID := baseID + "." + m.Name
				addEdge(fileID, mID, "declares")
			}

			for _, v := range file.Variables {
				varID := baseID + "." + v.Name
				allVariableIDs[varID] = struct{}{}

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

			for _, v := range file.Variables {
				if typ, ok := v.Params["type"]; ok {
					addEdge(baseID+"."+v.Name, baseID+"."+typ, "typed")
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
			}

			// Ensure declaration counts as a use for locals
			if _, isGlobal := globalVars[varID]; !isGlobal && len(v.Usages) == 0 {
				declaringFunc := toNodeID(pkg.Path) + "." + extractFunctionName(v.Scope)
				if declaringFunc != varID {
					addEdge(declaringFunc, varID, "uses")
				}
			}
		}

			processInvokes := func(opID string, usages []Usage) {
				for _, usage := range usages {
					if skipGlobalScope(usage.Scope) {
						continue
					}
					callerID := toNodeID(usage.Path) + "." + extractFunctionName(usage.Scope)
					if callerID != opID {
						addEdge(callerID, opID, "invokes")
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