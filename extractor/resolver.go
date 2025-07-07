package extractor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type SimplifiedASTNode struct {
	Type     string            `json:"type"`
	Name     string            `json:"name,omitempty"`
	Children []*SimplifiedASTNode `json:"children,omitempty"`
	Position *ASTNodePosition     `json:"position,omitempty"`
}

type ASTNodePosition struct {
	URI      string `json:"uri"`
	Line     int    `json:"line"`
	Character int   `json:"character"`
}

type ModifiedDefinitionInfo struct {
	Name      string
	URI       string
	Line      int
	Character int
	Kind      string
}

type DefinitionInfo struct {
	Name      string
	URI       string
	Line      int
	Character int
	Package   string
	Scope 	  string
}

type GoplsClient struct {
	stdin  io.WriteCloser
	stdout io.Reader
	seq    int
}

func NewGoplsClient(rootPath string) (*GoplsClient, error) {
	absPath, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	stdin, stdout, err := StartGopls(absPath)
	if err != nil {
		return nil, err
	}

	// Convert the rootPath to a file URI
	uri := "file://" + filepath.ToSlash(absPath)

	// Send initialize request
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"processId": nil,
			"rootUri":   uri,
			"capabilities": map[string]interface{}{},
			"workspaceFolders": []map[string]string{
				{
					"uri":  uri,
					"name": filepath.Base(rootPath),
				},
			},
		},
	}
	sendLSPMessage(stdin, initReq)

	// Wait for initialize response
	scanner := bufio.NewScanner(stdout)
	scanner.Split(splitLSP)
	if !scanner.Scan() {
		return nil, fmt.Errorf("no response from gopls after initialize")
	}
	// fmt.Printf("Initialize response: %s\n", scanner.Bytes())

	// Send initialized notification
	initNotif := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialized",
		"params":  map[string]interface{}{},
	}
	sendLSPMessage(stdin, initNotif)

	return &GoplsClient{
		stdin:  stdin,
		stdout: stdout,
		seq:    2, // next available ID
	}, nil
}

func (c *GoplsClient) Definition(uri string, line, character int) (*DefinitionInfo, error) {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      c.seq,
		"method":  "textDocument/definition",
		"params": map[string]interface{}{
			"textDocument": map[string]string{
				"uri": uri,
			},
			"position": map[string]int{
				"line":      line,
				"character": character,
			},
		},
	}
	c.seq++

	sendLSPMessage(c.stdin, req)

	// Read response (for simplicity assuming one message at a time)
	scanner := bufio.NewScanner(c.stdout)
	scanner.Split(splitLSP)
	if !scanner.Scan() {
		return nil, fmt.Errorf("no response from gopls")
	}

	// raw := scanner.Bytes()
	// fmt.Printf("Raw gopls response: %s\n", raw)

	var resp struct {
		Result []struct {
			URI   string `json:"uri"`
			Range struct {
				Start struct {
					Line      int `json:"line"`
					Character int `json:"character"`
				} `json:"start"`
			} `json:"range"`
		} `json:"result"`
	}

	err := json.Unmarshal(scanner.Bytes(), &resp)
	if err != nil {
		return nil, err
	}

	if len(resp.Result) == 0 {
		return nil, nil // not resolved
	}

	info := resp.Result[0]
	return &DefinitionInfo{
		URI:      info.URI,
		Line:     info.Range.Start.Line,
		Character: info.Range.Start.Character,
	}, nil
}

func ResolveNames(fset *token.FileSet, files map[string]*ast.File, rootDir string) (map[token.Pos]*DefinitionInfo, error) {
	client, err := NewGoplsClient(rootDir)
	if err != nil {
		return nil, err
	}

	resolved := make(map[token.Pos]*DefinitionInfo)

	for path, file := range files {
		absPath, err := filepath.Abs(path)
		if err != nil {
			fmt.Printf("failed to get absolute path for %s: %v\n", path, err)
			continue
		}
		uri := "file://" + filepath.ToSlash(absPath)

		var scopeStack []string

		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.FuncDecl:
				scopeStack = append(scopeStack, "func "+node.Name.Name)
			case *ast.TypeSpec:
				switch node.Type.(type) {
				case *ast.StructType:
					scopeStack = append(scopeStack, "struct "+node.Name.Name)
				case *ast.InterfaceType:
					scopeStack = append(scopeStack, "interface "+node.Name.Name)
				default:
					scopeStack = append(scopeStack, "type "+node.Name.Name)
				}
			}

			// Resolve identifier
			var ident *ast.Ident
			switch node := n.(type) {
			case *ast.Ident:
				ident = node
			case *ast.SelectorExpr:
				ident = node.Sel
			case *ast.CallExpr:
				if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
					ident = sel.Sel
				} else if id, ok := node.Fun.(*ast.Ident); ok {
					ident = id
				}
			}

			if ident != nil {
				pos := fset.Position(ident.Pos())
				info, err := client.Definition(uri, pos.Line-1, pos.Column-1)
				if err == nil && info != nil {
					info.Name = ident.Name
					info.Package = file.Name.Name

					if len(scopeStack) > 0 {
						info.Scope = scopeStack[len(scopeStack)-1] // top of stack
					} else {
						info.Scope = "global"
					}

					resolved[ident.Pos()] = info
				}
			}

			return true
		})

		// Use defer-like post-order to pop scope
		ast.Inspect(file, func(n ast.Node) bool {
			switch n.(type) {
			case *ast.FuncDecl, *ast.TypeSpec:
				if len(scopeStack) > 0 {
					scopeStack = scopeStack[:len(scopeStack)-1]
				}
			}
			return true
		})
	}

	return resolved, nil
}

func buildSimplifiedAST(fset *token.FileSet, node ast.Node, path string) *SimplifiedASTNode {
	if node == nil {
		return nil
	}

	var simp *SimplifiedASTNode
	var children []*SimplifiedASTNode

	addChild := func(n ast.Node) {
		if n == nil {
			return
		}
		if child := buildSimplifiedAST(fset, n, path); child != nil {
			children = append(children, child)
		}
	}

	switch n := node.(type) {

	case *ast.File:
		simp = newNode("File", filepath.Base(path), fset, path, n.Pos())
		if n.Name != nil {
			pkgNode := newNode("Package", n.Name.Name, fset, path, n.Name.Pos())
			children = append(simp.Children, pkgNode)
		}
		for _, decl := range n.Decls {
			addChild(decl)
		}

	case *ast.ImportSpec:
		var importPath string
		if n.Path != nil {
			importPath = strings.Trim(n.Path.Value, `"`)
		}
		simp = newNode("ImportSpec", importPath, fset, path, n.Pos())

	case *ast.GenDecl:
		simp = newNode("GenDecl", "", fset, path, n.Pos())
		for _, spec := range n.Specs {
			addChild(spec)
		}

	case *ast.FuncDecl:
		simp = newNode("FuncDecl", n.Name.Name, fset, path, n.Pos())

		if n.Recv != nil {
			recvWrapper := newNode("Receiver", "", fset, path, n.Recv.Pos())
			if recv := buildSimplifiedAST(fset, n.Recv, path); recv != nil {
				recvWrapper.Children = []*SimplifiedASTNode{recv}
			}
			children = append(children, recvWrapper)
		}

		if n.Type != nil {
			addChild(n.Type)
		}

		if n.Body != nil {
			addChild(n.Body)
		}

	case *ast.FuncType:
		simp = newNode("FuncType", "", fset, path, n.Pos())

		if n.Results != nil {
			resultWrapper := newNode("Results", "", fset, path, n.Results.Pos())
			if result := buildSimplifiedAST(fset, n.Results, path); result != nil {
				resultWrapper.Children = []*SimplifiedASTNode{result}
			}
			children = append(children, resultWrapper)
		}

	case *ast.TypeSpec:
		simp = newNode("TypeSpec", n.Name.Name, fset, path, n.Pos())
		addChild(n.Type)

	case *ast.StructType:
		simp = newNode("StructType", "", fset, path, n.Pos())
		if n.Fields != nil {
			for _, field := range n.Fields.List {
				addChild(field)
			}
		}

	case *ast.FieldList:
		simp = newNode("FieldList", "", fset, path, n.Pos())
		for _, field := range n.List {
			addChild(field)
		}

	case *ast.Field:
		simp = newNode("Field", "", fset, path, n.Pos())
		for _, name := range n.Names {
			addChild(name)
		}
		addChild(n.Type)

	case *ast.BlockStmt:
		simp = newNode("BlockStmt", "", fset, path, n.Pos())
		for _, stmt := range n.List {
			addChild(stmt)
		}

	case *ast.AssignStmt:
		simp = newNode("AssignStmt", "", fset, path, n.Pos())
		for _, lhs := range n.Lhs {
			addChild(lhs)
		}
		for _, rhs := range n.Rhs {
			addChild(rhs)
		}

	case *ast.CompositeLit:
		simp = newNode("CompositeLit", "", fset, path, n.Pos())
		addChild(n.Type)
		for _, elt := range n.Elts {
			addChild(elt)
		}

	case *ast.KeyValueExpr:
		simp = newNode("KeyValueExpr", "", fset, path, n.Pos())
		addChild(n.Key)
		addChild(n.Value)

	case *ast.Ident:
		simp = newNode("Ident", n.Name, fset, path, n.Pos())

	case *ast.BasicLit:
		simp = newNode("BasicLit", n.Value, fset, path, n.Pos())

	case *ast.CallExpr:
		simp = newNode("CallExpr", "", fset, path, n.Pos())
		addChild(n.Fun)
		for _, arg := range n.Args {
			addChild(arg)
		}

	case *ast.SelectorExpr:
		simp = newNode("SelectorExpr", "", fset, path, n.Pos())
		addChild(n.X)
		addChild(n.Sel)

	case *ast.ValueSpec:
		simp = newNode("ValueSpec", "", fset, path, n.Pos())
		for _, name := range n.Names {
			addChild(name)
		}
		addChild(n.Type)
		for _, v := range n.Values {
			addChild(v)
		}

	case *ast.ReturnStmt:
		simp = newNode("ReturnStmt", "", fset, path, n.Pos())
		for _, r := range n.Results {
			addChild(r)
		}

	default:
		simp = newNode(fmt.Sprintf("%T", n), "", fset, path, n.Pos())
	}

	if simp != nil {
		simp.Children = children
	}

	return simp
}

func BuildSimplifiedASTs(fset *token.FileSet, files map[string]*ast.File) map[string]*SimplifiedASTNode {
	asts := make(map[string]*SimplifiedASTNode)

	for path, file := range files {
		asts[path] = buildSimplifiedAST(fset, file, path)
	}

	return asts
}

func newNode(kind, name string, fset *token.FileSet, path string, pos token.Pos) *SimplifiedASTNode {
	position := fset.Position(pos)
	return &SimplifiedASTNode{
		Type: kind,
		Name: name,
		Position: &ASTNodePosition{
			URI:       "file://" + filepath.ToSlash(path),
			Line:      position.Line - 1,
			Character: position.Column - 1,
		},
	}
}

func OutputSimplifiedASTs(fset *token.FileSet, files map[string]*ast.File, projectRoot string, outDir string) error {
	asts := BuildSimplifiedASTs(fset, files)

	for path, astNode := range asts {
		absPath, err := filepath.Abs(path) // Ensure absolute path
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(projectRoot, absPath)
		if err != nil {
			return fmt.Errorf("cannot get relative path from %s to %s: %w", projectRoot, absPath, err)
		}

		jsonFileName := relPath[:len(relPath)-len(filepath.Ext(relPath))] + ".simplified.json"
		outputPath := filepath.Join(outDir, filepath.ToSlash(jsonFileName)) // Normalize slashes

		err = os.MkdirAll(filepath.Dir(outputPath), os.ModePerm)
		if err != nil {
			return err
		}

		f, err := os.Create(outputPath)
		if err != nil {
			return err
		}
		defer f.Close()

		encoder := json.NewEncoder(f)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(astNode); err != nil {
			return err
		}
	}

	return nil
}

func CollectSymbolTable(ast *SimplifiedASTNode) map[string]*ModifiedDefinitionInfo {
	symbols := make(map[string]*ModifiedDefinitionInfo)

	var walk func(node *SimplifiedASTNode)
	walk = func(node *SimplifiedASTNode) {
		if node == nil {
			return
		}

		switch node.Type {

		case "FuncDecl":
			symbols[node.Name] = &ModifiedDefinitionInfo{
				Name:      node.Name,
				URI:       node.Position.URI,
				Line:      node.Position.Line,
				Character: node.Position.Character,
				Kind:      "func",
			}

			for _, child := range node.Children {
				if child.Type == "BlockStmt" {
					collectSymbolsFromBlock(child, symbols)
				}
			}

		case "TypeSpec":
			if len(node.Children) > 0 {
				child := node.Children[0]
				switch child.Type {
				case "StructType":
					symbols[node.Name] = &ModifiedDefinitionInfo{
						Name:      node.Name,
						URI:       node.Position.URI,
						Line:      node.Position.Line,
						Character: node.Position.Character,
						Kind:      "struct",
					}
				case "InterfaceType":
					symbols[node.Name] = &ModifiedDefinitionInfo{
						Name:      node.Name,
						URI:       node.Position.URI,
						Line:      node.Position.Line,
						Character: node.Position.Character,
						Kind:      "interface",
					}
				}
			}

		case "ValueSpec":
			for _, field := range node.Children {
				if field.Type == "Ident" {
					symbols[field.Name] = &ModifiedDefinitionInfo{
						Name:      field.Name,
						URI:       field.Position.URI,
						Line:      field.Position.Line,
						Character: field.Position.Character,
						Kind:      "var",
					}
				}
			}
		}

		for _, child := range node.Children {
			walk(child)
		}
	}

	walk(ast)
	return symbols
}

func LoadSimplifiedASTs(dir string) (map[string]*SimplifiedASTNode, error) {
	simplifiedASTs := make(map[string]*SimplifiedASTNode)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip non-simplified JSON files
		if !strings.HasSuffix(path, ".simplified.json") {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", path, err)
		}
		defer f.Close()

		var root SimplifiedASTNode
		decoder := json.NewDecoder(f)
		if err := decoder.Decode(&root); err != nil {
			return fmt.Errorf("failed to decode file %s: %w", path, err)
		}

		simplifiedASTs[path] = &root
		return nil
	})

	if err != nil {
		return nil, err
	}

	return simplifiedASTs, nil
}

func collectSymbolsFromBlock(node *SimplifiedASTNode, table map[string]*ModifiedDefinitionInfo) {
	if node.Type == "BlockStmt" {
		for _, stmt := range node.Children {
			switch stmt.Type {
			case "AssignStmt":
				// Look for short variable declarations (:=)
				for _, lhs := range stmt.Children {
					if lhs.Type == "Ident" {
						table[lhs.Name] = &ModifiedDefinitionInfo{
							Name: lhs.Name,
							Kind: "var",
							URI:  lhs.Position.URI,
							Line: lhs.Position.Line,
							Character: lhs.Position.Character,
						}
					}
				}
			case "DeclStmt":
				for _, decl := range stmt.Children {
					if decl.Type == "GenDecl" {
						for _, spec := range decl.Children {
							if spec.Type == "ValueSpec" {
								for _, id := range spec.Children {
									if id.Type == "Ident" {
										table[id.Name] = &ModifiedDefinitionInfo{
											Name: id.Name,
											Kind: "var",
											URI:  id.Position.URI,
											Line: id.Position.Line,
											Character: id.Position.Character,
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// Recurse into children
	for _, child := range node.Children {
		collectSymbolsFromBlock(child, table)
	}
}
