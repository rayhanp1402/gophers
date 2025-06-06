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

		// Track root-level directories and files for "includes" edges
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

			// Build set of root-level directories and files (for "includes")
			if folderPath == "" {
				// Root-level file
				rootFiles[pkg.Path] = struct{}{}
			} else {
				// Root-level directory (folderPath itself is root dir)
				rootDirs[folderPath] = struct{}{}
			}

			// Edge: package declared in file
			addEdge(fileID, packageID, "declares")

			// Edge: folder contains file
			addEdge(folderID, fileID, "contains")

			// Structs & Interfaces edges
			for _, s := range file.Structs {
				typeNodeID := baseID + "." + s.Name
				addEdge(packageID, typeNodeID, "encloses")
			}

			for _, iface := range file.Interfaces {
				typeNodeID := baseID + "." + iface.Name
				addEdge(packageID, typeNodeID, "encloses")
			}

			// Function calls
			for _, fn := range file.Functions {
				targetID := baseID + "." + fn.Name
				for _, usage := range fn.Usages {
					if usage.Path == pkg.Path && extractFunctionName(usage.Scope) == fn.Name {
						continue
					}
					if skipGlobalScope(usage.Scope) {
						continue
					}
					sourceID := toNodeID(usage.Path) + "." + extractFunctionName(usage.Scope)
					addEdge(sourceID, targetID, "calls")
				}
			}

			// Variables usages
			for _, variable := range file.Variables {
				targetID := baseID + "." + variable.Name
				for _, usage := range variable.Usages {
					if usage.Path == pkg.Path && extractFunctionName(usage.Scope) == variable.Name {
						continue
					}
					if skipGlobalScope(usage.Scope) {
						continue
					}
					sourceID := toNodeID(usage.Path) + "." + extractFunctionName(usage.Scope)
					addEdge(sourceID, targetID, "holds")
				}
			}

			// Struct usages
			for _, strct := range file.Structs {
				targetID := baseID + "." + strct.Name
				for _, usage := range strct.Usages {
					if usage.Path == pkg.Path && extractFunctionName(usage.Scope) == strct.Name {
						continue
					}
					if skipGlobalScope(usage.Scope) {
						continue
					}
					sourceID := toNodeID(usage.Path) + "." + extractFunctionName(usage.Scope)
					addEdge(sourceID, targetID, "holds")
				}
			}

			// Interface usages
			for _, iface := range file.Interfaces {
				targetID := baseID + "." + iface.Name
				for _, usage := range iface.Usages {
					if usage.Path == pkg.Path && extractFunctionName(usage.Scope) == iface.Name {
						continue
					}
					if skipGlobalScope(usage.Scope) {
						continue
					}
					sourceID := toNodeID(usage.Path) + "." + extractFunctionName(usage.Scope)
					addEdge(sourceID, targetID, "holds")
				}
			}
		}

		// Add includes edges from project to root-level directories
		for dir := range rootDirs {
			dirID := toNodeID(dir)
			addEdge(projectID, dirID, "includes")
		}

		// Add includes edges from project to root-level files
		for file := range rootFiles {
			fileID := toNodeID(file)
			addEdge(projectID, fileID, "includes")
		}
	}

	return edges
}

