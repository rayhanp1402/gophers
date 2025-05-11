package extractor

import (
	"fmt"
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

func GenerateNodes(pkgs []PackageNode) []GraphNode {
	var nodes []GraphNode
	idCounter := 1

	for _, pkg := range pkgs {
		file := pkg.File
		baseID := toNodeID(pkg.Path)

		// Add structs
		for _, s := range file.Structs {
			node := GraphNode{
				Data: NodeData{
					ID:     baseID + "." + s.Name,
					Labels: []string{"Structure"},
					Properties: map[string]string{
						"simpleName": s.Name,
						"kind":       "struct",
					},
				},
			}
			idCounter++
			nodes = append(nodes, node)
		}

		// Add interfaces
		for _, iface := range file.Interfaces {
			node := GraphNode{
				Data: NodeData{
					ID:     baseID + "." + iface.Name,
					Labels: []string{"Structure"},
					Properties: map[string]string{
						"simpleName": iface.Name,
						"kind":       "interface",
					},
				},
			}
			idCounter++
			nodes = append(nodes, node)
		}

		// Add functions
		for _, fn := range file.Functions {
			node := GraphNode{
				Data: NodeData{
					ID:     baseID + "." + fn.Name,
					Labels: []string{"Structure"},
					Properties: map[string]string{
						"simpleName": fn.Name,
						"kind":       "function",
					},
				},
			}
			idCounter++
			nodes = append(nodes, node)
		}

		// Add methods
		for _, m := range file.Methods {
			node := GraphNode{
				Data: NodeData{
					ID:     baseID + "." + m.Name,
					Labels: []string{"Structure"},
					Properties: map[string]string{
						"simpleName": m.Name,
						"kind":       "function",
					},
				},
			}
			idCounter++
			nodes = append(nodes, node)
		}

		// Add variables
		for _, v := range file.Variables {
			node := GraphNode{
				Data: NodeData{
					ID:     baseID + "." + v.Name,
					Labels: []string{"Structure"},
					Properties: map[string]string{
						"simpleName": v.Name,
						"kind":       "variable",
					},
				},
			}
			nodes = append(nodes, node)
		}
	}

	return nodes
}

func GenerateEdges(pkgs []PackageNode) []GraphEdge {
	var edges []GraphEdge
	counter := 1

	skipGlobalScope := func(scope string) bool {
		return extractFunctionName(scope) == "global"
	}

	for _, pkg := range pkgs {
		file := pkg.File
		baseID := toNodeID(pkg.Path)

		// Process function calls
		for _, fn := range file.Functions {
			targetID := baseID + "." + fn.Name

			for _, usage := range fn.Usages {
				if usage.Path == pkg.Path && extractFunctionName(usage.Scope) == fn.Name {
					continue // skip declaration usage
				}
				if skipGlobalScope(usage.Scope) {
					continue // temporarily skip global scope
				}

				sourceID := toNodeID(usage.Path) + "." + extractFunctionName(usage.Scope)

				edges = append(edges, GraphEdge{
					Data: EdgeData{
						ID:     fmt.Sprintf("edge%d", counter),
						Label:  "calls",
						Source: sourceID,
						Target: targetID,
						Properties: map[string]string{},
					},
				})
				counter++
			}
		}

		// Process variables with "holds" edges
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

				edges = append(edges, GraphEdge{
					Data: EdgeData{
						ID:     fmt.Sprintf("edge%d", counter),
						Label:  "holds",
						Source: sourceID,
						Target: targetID,
						Properties: map[string]string{},
					},
				})
				counter++
			}
		}

		// Process structs with "holds" edges
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

				edges = append(edges, GraphEdge{
					Data: EdgeData{
						ID:     fmt.Sprintf("edge%d", counter),
						Label:  "holds",
						Source: sourceID,
						Target: targetID,
						Properties: map[string]string{},
					},
				})
				counter++
			}
		}

		// Process interfaces with "holds" edges
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

				edges = append(edges, GraphEdge{
					Data: EdgeData{
						ID:     fmt.Sprintf("edge%d", counter),
						Label:  "holds",
						Source: sourceID,
						Target: targetID,
						Properties: map[string]string{},
					},
				})
				counter++
			}
		}
	}

	return edges
}


