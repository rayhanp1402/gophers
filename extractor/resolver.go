package extractor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"path/filepath"
)

type DefinitionInfo struct {
	Name     string
	URI      string
	Line     int
	Character int
	Package   string
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

	stdin, stdout, err := StartGopls(rootPath)
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
		}
		uri := "file://" + filepath.ToSlash(absPath)

		// fmt.Printf("Processing file: %s (URI: %s)\n", path, uri)

		ast.Inspect(file, func(n ast.Node) bool {
			ident, ok := n.(*ast.Ident)
			if !ok || ident.Obj == nil {
				return true
			}

			// fmt.Printf("Found identifier: %s at position: %v\n", ident.Name, fset.Position(ident.Pos()))

			pos := fset.Position(ident.Pos())
			info, err := client.Definition(uri, pos.Line-1, pos.Column-1)
			if err == nil && info != nil {
				// fmt.Printf("Resolved definition for identifier %s: %+v\n", ident.Name, info)

				info.Name = ident.Name
				info.Package = file.Name.Name
				resolved[ident.Pos()] = info
			} else {
				// fmt.Printf("Failed to resolve definition for %s: %v\n", ident.Name, err)
			}
			return true
		})
	}

	return resolved, nil
}
