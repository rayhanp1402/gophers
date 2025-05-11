package extractor

import "strings"

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
		baseID := strings.TrimLeft(strings.TrimSuffix(strings.ReplaceAll(pkg.Path, "\\", "."), ".go"), ".")

		// Add structs
		for _, s := range file.Structs {
			node := GraphNode{
				Data: NodeData{
					ID:     baseID + "." + s.Name,
					Labels: []string{"Type"},
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
					Labels: []string{"Type"},
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
					Labels: []string{"Operation"},
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
					Labels: []string{"Operation"},
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
					Labels: []string{"Type"},
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
	return []GraphEdge{}
}