package extractor

import (
	"encoding/json"
	"go/token"
)

type ASTNode struct {
	Name       string       `json:"name"`
	Type       string       `json:"type"`
	Position   token.Position `json:"position"`
	Definition *ASTNode     `json:"-"`
	References []*ASTNode   `json:"references,omitempty"`
}

func (n *ASTNode) MarshalJSON() ([]byte, error) {
	type Alias ASTNode
	return json.Marshal(&struct {
		Definition *string `json:"definition,omitempty"`
		*Alias
	}{
		Definition: func() *string {
			if n.Definition != nil {
				name := n.Definition.Name
				return &name
			}
			return nil
		}(),
		Alias: (*Alias)(n),
	})
}