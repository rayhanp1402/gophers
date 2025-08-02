package extractor

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/importer"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

type SimplifiedASTNode struct {
	Type     string            `json:"type"`
	Name     string            `json:"name,omitempty"`
	Children []*SimplifiedASTNode `json:"children,omitempty"`
	Position *ASTNodePosition     `json:"position,omitempty"`
	DeclaredAt *ModifiedDefinitionInfo    `json:"declaredAt,omitempty"`
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
	Type      string
	ReceiverType string
	PackageName  string
}

func LoadTypesInfo(
	fset *token.FileSet,
	files map[string]*ast.File,
	absPath string,
) (*types.Info, *types.Package, error) {
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedImports | packages.NeedTypes,
		Fset:  fset,
		Dir:   absPath,
		Tests: false,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil || len(pkgs) == 0 {
		return nil, nil, fmt.Errorf("failed to load packages: %w", err)
	}

	importer := importer.ForCompiler(fset, "source", nil)

	filesByPkg := map[string][]*ast.File{}
	for _, f := range files {
		pkgName := f.Name.Name
		filesByPkg[pkgName] = append(filesByPkg[pkgName], f)
	}

	mergedInfo := &types.Info{
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}

	var lastPkg *types.Package

	for pkgName, fileList := range filesByPkg {
		info := &types.Info{
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
		}

		config := &types.Config{
			Importer:                 importer,
			DisableUnusedImportCheck: true,
			Error: func(err error) {
				log.Printf("type error (%s): %v", pkgName, err)
			},
		}

		pkg, err := config.Check(pkgName, fset, fileList, info)
		if err != nil {
			log.Printf("type checking failed for package %s: %v", pkgName, err)
			continue
		}

		lastPkg = pkg

		for k, v := range info.Defs {
			mergedInfo.Defs[k] = v
		}
		for k, v := range info.Uses {
			mergedInfo.Uses[k] = v
		}
		for k, v := range info.Selections {
			mergedInfo.Selections[k] = v
		}
	}

	if lastPkg == nil {
		return nil, nil, fmt.Errorf("type checking failed for all packages")
	}

	return mergedInfo, lastPkg, nil
}

func buildSimplifiedASTWithGlobals(
	fset *token.FileSet,
	node ast.Node,
	path string,
	globalVars map[string]struct{},
	typesInfo *types.Info,
) *SimplifiedASTNode {
	if node == nil {
		return nil
	}

	var simp *SimplifiedASTNode
	var children []*SimplifiedASTNode

	addChild := func(n ast.Node) {
		if n == nil {
			return
		}
		if child := buildSimplifiedASTWithGlobals(fset, n, path, globalVars, typesInfo); child != nil {
			if child.Type == "" && child.Name == "" && len(child.Children) > 0 {
				children = append(children, child.Children...)
			} else {
				children = append(children, child)
			}
		}
	}

	switch n := node.(type) {

	case *ast.File:
		simp = newNode("File", filepath.Base(path), fset, path, n.Pos(), nil)
		if n.Name != nil {
			pkgNode := newNode("Package", n.Name.Name, fset, path, n.Name.Pos(), typesInfo.ObjectOf(n.Name))
			children = append(children, pkgNode)
		}
		for _, decl := range n.Decls {
			addChild(decl)
		}

	case *ast.GenDecl:
		var specs []*SimplifiedASTNode
		for _, spec := range n.Specs {
			if child := buildSimplifiedASTWithGlobals(fset, spec, path, globalVars, typesInfo); child != nil {
				specs = append(specs, child)
			}
		}
		return &SimplifiedASTNode{Children: specs}

	case *ast.FuncDecl:
		fmt.Println("Processing Function:", n.Name.Name)
		nodeType := "Function"
		if n.Recv != nil {
			nodeType = "Method"
		}
		simp = newNode(nodeType, n.Name.Name, fset, path, n.Pos(), typesInfo.ObjectOf(n.Name))

		if n.Recv != nil {
			recvWrapper := newNode("Receiver", "", fset, path, n.Recv.Pos(), nil)
			if recv := buildSimplifiedASTWithGlobals(fset, n.Recv, path, globalVars, typesInfo); recv != nil {
				recvWrapper.Children = []*SimplifiedASTNode{recv}
			}
			children = append(children, recvWrapper)
		}

		if n.Type != nil {
			if n.Type.Params != nil {
				paramWrapper := newNode("Params", "", fset, path, n.Type.Params.Pos(), nil)
				for _, field := range n.Type.Params.List {
					if param := buildSimplifiedASTWithGlobals(fset, field, path, globalVars, typesInfo); param != nil {
						paramWrapper.Children = append(paramWrapper.Children, param)
					}
				}
				children = append(children, paramWrapper)
			}

			if n.Type.Results != nil {
				resultWrapper := newNode("Results", "", fset, path, n.Type.Results.Pos(), nil)
				for _, field := range n.Type.Results.List {
					if result := buildSimplifiedASTWithGlobals(fset, field, path, globalVars, typesInfo); result != nil {
						resultWrapper.Children = append(resultWrapper.Children, result)
					}
				}
				children = append(children, resultWrapper)
			}
		}

		if n.Body != nil {
			handled := map[token.Pos]bool{}

			ast.Inspect(n.Body, func(x ast.Node) bool {
				switch expr := x.(type) {

				case *ast.CallExpr:
					switch fun := expr.Fun.(type) {
					case *ast.Ident:
						obj := typesInfo.ObjectOf(fun)
						children = append(children, newNode("Call", fun.Name, fset, path, fun.Pos(), obj))
					case *ast.SelectorExpr:
						obj := typesInfo.ObjectOf(fun.Sel)
						children = append(children, newNode("MethodCall", fun.Sel.Name, fset, path, fun.Sel.Pos(), obj))
						handled[fun.Sel.Pos()] = true
					}

				case *ast.Ident:
					obj := typesInfo.ObjectOf(expr)
					if obj != nil {
						if _, ok := globalVars[expr.Name]; ok {
							children = append(children, newNode("GlobalVarUse", expr.Name, fset, path, expr.Pos(), obj))
						} else if v, ok := obj.(*types.Var); ok && !v.IsField() {
							children = append(children, newNode("VarUse", expr.Name, fset, path, expr.Pos(), obj))
						}
					}

				case *ast.CompositeLit:
					switch t := expr.Type.(type) {
					case *ast.SelectorExpr:
						obj := typesInfo.ObjectOf(t.Sel)
						children = append(children, newNode("TypeUse", t.Sel.Name, fset, path, t.Sel.Pos(), obj))
						handled[t.Sel.Pos()] = true
					case *ast.Ident:
						obj := typesInfo.ObjectOf(t)
						children = append(children, newNode("TypeUse", t.Name, fset, path, t.Pos(), obj))
						handled[t.Pos()] = true
					}

				case *ast.SelectorExpr:
					if handled[expr.Sel.Pos()] {
						return false
					}

					if selInfo, ok := typesInfo.Selections[expr]; ok {
						obj := selInfo.Obj()
						kind := "FieldUse"
						if selInfo.Kind() == types.MethodVal || selInfo.Kind() == types.MethodExpr {
							kind = "MethodCall"
						}

						node := newResolvedNode(kind, expr.Sel.Name, fset, path, expr.Sel.Pos(), obj)

						children = append(children, node)
					} else {
						// fallback to ObjectOf if Selection failed
						obj := typesInfo.ObjectOf(expr.Sel)

						kind := "FieldUse"
						if obj != nil {
							switch v := obj.(type) {
							case *types.TypeName:
								kind = "TypeUse"
							case *types.Var:
								if v.IsField() {
									kind = "FieldUse"
								} else if v.Pkg() != nil {
									kind = "GlobalVarUse"
								} else {
									kind = "VarUse"
								}
							case *types.Func:
								kind = "MethodCall"
							}
						}

						node := newResolvedNode(kind, expr.Sel.Name, fset, path, expr.Sel.Pos(), obj)
						if node.DeclaredAt != nil {
							log.Printf("  -> DeclaredAt: %+v\n", *node.DeclaredAt)
						} else {
							log.Println("  -> DeclaredAt: nil")
						}

						children = append(children, node)
					}

					handled[expr.Sel.Pos()] = true
					return false
				}
				return true
			})
		}

	case *ast.TypeSpec:
		if n.Assign != token.NoPos {
			fmt.Println("Skipping alias:", n.Name.Name)
			return nil
		}
		fmt.Println("Processing TypeSpec:", n.Name.Name)
		obj := typesInfo.ObjectOf(n.Name)
		switch actual := n.Type.(type) {
		case *ast.StructType:
			simp = newNode("Struct", n.Name.Name, fset, path, n.Pos(), obj)
			addChild(actual)
		case *ast.InterfaceType:
			simp = newNode("Interface", n.Name.Name, fset, path, n.Pos(), obj)
			addChild(actual)
		default:
			simp = newNode("Type", n.Name.Name, fset, path, n.Pos(), obj)
			addChild(actual)
		}

	case *ast.StructType:
		fmt.Println("Processing StructType")
		simp = newNode("Struct", "", fset, path, n.Pos(), nil)
		if n.Fields != nil {
			for _, field := range n.Fields.List {
				addChild(field)
			}
		}

	case *ast.InterfaceType:
		fmt.Println("Processing InterfaceType")
		simp = newNode("Interface", "", fset, path, n.Pos(), nil)
		if n.Methods != nil {
			for _, field := range n.Methods.List {
				addChild(field)
			}
		}

	case *ast.FieldList:
		simp = newNode("FieldList", "", fset, path, n.Pos(), nil)
		for _, field := range n.List {
			addChild(field)
		}

	case *ast.Field:
		simp = newNode("Field", "", fset, path, n.Pos(), nil)

		// Add any field names (e.g., for parameters or named return values)
		for _, name := range n.Names {
			addChild(name)
		}

		// Add the type node, even if the field is unnamed
		if n.Type != nil {
			switch typ := n.Type.(type) {
			case *ast.Ident:
				// Built-in or local type
				typeNode := newNode("Ident", typ.Name, fset, path, typ.Pos(), typesInfo.ObjectOf(typ))
				children = append(children, typeNode)
			case *ast.SelectorExpr:
				// Imported type (e.g., models.CalculationResult)
				if sel := typ.Sel; sel != nil {
					typeNode := newNode("SelectorExpr", fmt.Sprintf("%s.%s", renderExpr(typ.X), sel.Name), fset, path, sel.Pos(), typesInfo.ObjectOf(sel))
					children = append(children, typeNode)
				}
			default:
				addChild(n.Type) // fallback for complex cases (arrays, pointers, etc.)
			}
		}

	case *ast.ValueSpec:
		fmt.Println("Processing GlobalVar")
		simp = newNode("GlobalVar", "", fset, path, n.Pos(), nil)
		for _, name := range n.Names {
			addChild(name)
			globalVars[name.Name] = struct{}{}
		}
		addChild(n.Type)

	case *ast.Ident:
		simp = newNode("Ident", n.Name, fset, path, n.Pos(), typesInfo.ObjectOf(n))
		
	case *ast.ImportSpec:
		importPath, err := strconv.Unquote(n.Path.Value)
		if err != nil {
			importPath = n.Path.Value // fallback if not quoted
		}
		simp = newNode("Import", importPath, fset, path, n.Pos(), nil)

	default:
		return nil
	}

	if simp != nil {
		simp.Children = children
	}
	return simp
}

func BuildSimplifiedASTs(
	fset *token.FileSet,
	files map[string]*ast.File,
	typesInfo *types.Info,
) map[string]*SimplifiedASTNode {
	asts := make(map[string]*SimplifiedASTNode)
	globalVars := make(map[string]struct{})

	// First pass: collect all global variable names from all files
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			if vspec, ok := n.(*ast.ValueSpec); ok {
				for _, name := range vspec.Names {
					globalVars[name.Name] = struct{}{}
				}
			}
			return true
		})
	}

	// Second pass: generate simplified ASTs using the collected global variables
	for path, file := range files {
		asts[path] = buildSimplifiedASTWithGlobals(fset, file, path, globalVars, typesInfo)
	}

	return asts
}

func newNode(kind, name string, fset *token.FileSet, path string, pos token.Pos, obj types.Object) *SimplifiedASTNode {
	position := fset.Position(pos)
	absPath, _ := filepath.Abs(path)

	var declaredAt *ModifiedDefinitionInfo
	if obj != nil {
		objPos := fset.Position(obj.Pos())
		objAbsPath, _ := filepath.Abs(objPos.Filename)

		pkgName := ""
		if obj.Pkg() != nil {
			pkgName = obj.Pkg().Name()
		}

		declaredAt = &ModifiedDefinitionInfo{
			Name:         obj.Name(),
			URI:          "file://" + filepath.ToSlash(objAbsPath),
			Line:         objPos.Line - 1,
			Character:    objPos.Column - 1,
			Kind:         strings.ToLower(strings.TrimPrefix(fmt.Sprintf("%T", obj), "*types.")),
			Type:         obj.Type().String(),
			ReceiverType: receiverTypeString(obj),
			PackageName:  pkgName,
		}
	}

	return &SimplifiedASTNode{
		Type: kind,
		Name: name,
		Position: &ASTNodePosition{
			URI:       "file://" + filepath.ToSlash(absPath),
			Line:      position.Line - 1,
			Character: position.Column - 1,
		},
		DeclaredAt: declaredAt,
	}
}

func receiverTypeString(obj types.Object) string {
	if fn, ok := obj.(*types.Func); ok {
		if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
			return sig.Recv().Type().String()
		}
	}
	return ""
}

func newResolvedNode(kind, name string, fset *token.FileSet, path string, pos token.Pos, obj types.Object) *SimplifiedASTNode {
	node := newNode(kind, name, fset, path, pos, obj)
	if obj != nil {
		position := fset.Position(obj.Pos())

		absPath, err := filepath.Abs(position.Filename)
		if err != nil {
			absPath = position.Filename
		}

		pkgName := ""
		if obj.Pkg() != nil {
			pkgName = obj.Pkg().Name()
		}

		node.DeclaredAt = &ModifiedDefinitionInfo{
			Name:         obj.Name(),
			URI:          "file://" + filepath.ToSlash(absPath),
			Line:         position.Line,
			Character:    position.Column,
			Kind:         objectKind(obj),
			Type:         obj.Type().String(),
			ReceiverType: receiverType(obj),
			PackageName:  pkgName,
		}
	}
	return node
}

func objectKind(obj types.Object) string {
	switch obj := obj.(type) {
	case *types.Const:
		return "const"
	case *types.Var:
		if obj.IsField() {
			return "field"
		}
		return "var"
	case *types.Func:
		return "func"
	case *types.TypeName:
		return "type"
	case *types.Label:
		return "label"
	case *types.PkgName:
		return "package"
	case *types.Builtin:
		return "builtin"
	}
	return "unknown"
}

func receiverType(obj types.Object) string {
	if fn, ok := obj.(*types.Func); ok {
		if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
			return sig.Recv().Type().String()
		}
	}
	return ""
}

func renderExpr(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	default:
		return "<unknown>"
	}
}

func OutputSimplifiedASTs(fset *token.FileSet, files map[string]*ast.File, projectRoot string, outDir string, typesInfo *types.Info) error {
	asts := BuildSimplifiedASTs(fset, files, typesInfo)

	for path, astNode := range asts {
		absPath, err := filepath.Abs(path)
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

	var walk func(node *SimplifiedASTNode, parentType string)

	packageName := ""
	if ast != nil && len(ast.Children) > 0 && ast.Children[0].Type == "Package" {
		packageName = ast.Children[0].Name
	}

	walk = func(node *SimplifiedASTNode, parentType string) {
		if node == nil || node.Position == nil {
			return
		}

		posKey := fmt.Sprintf("%s:%d:%d", node.Position.URI, node.Position.Line, node.Position.Character)

		switch node.Type {
		case "Function", "Method":
			kind := "func"
			var receiverType string

			if node.Type == "Method" {
				kind = "method"
				// look for Receiver node
				for _, child := range node.Children {
					if child.Type == "Receiver" {
						for _, fieldList := range child.Children {
							if fieldList.Type == "FieldList" {
								for _, field := range fieldList.Children {
									if field.Type == "Field" {
										for _, ident := range field.Children {
											if ident.Type == "Ident" {
												receiverType = ident.Name
											}
										}
									}
								}
							}
						}
					}
				}
			}

			symbols[posKey] = &ModifiedDefinitionInfo{
				Name:         node.Name,
				Kind:         kind,
				URI:          node.Position.URI,
				Line:         node.Position.Line,
				Character:    node.Position.Character,
				ReceiverType: receiverType,
			}

		case "Params":
			for _, field := range node.Children {
				if field.Type == "Field" {
					var fieldType string

					// Find the type from Field's children (e.g., Ident or SelectorExpr)
					for _, sub := range field.Children {
						if sub.Type == "Ident" || sub.Type == "SelectorExpr" {
							fieldType = sub.Name
						}
					}

					// Now find the identifier(s) that use this type
					for _, sub := range field.Children {
						if sub.Type == "Ident" && sub.Position != nil {
							// Skip if it's the type, already used
							if sub.Name == fieldType {
								continue
							}

							identKey := fmt.Sprintf("%s:%d:%d", sub.Position.URI, sub.Position.Line, sub.Position.Character)
							symbols[identKey] = &ModifiedDefinitionInfo{
								Name:      sub.Name,
								Kind:      "param",
								Type:      fieldType,
								URI:       sub.Position.URI,
								Line:      sub.Position.Line,
								Character: sub.Position.Character,
							}
						}
					}
				}
			}

		case "GlobalVar":
			for _, child := range node.Children {
				if child.Type == "Ident" {
					childKey := fmt.Sprintf("%s:%d:%d", child.Position.URI, child.Position.Line, child.Position.Character)
					symbols[childKey] = &ModifiedDefinitionInfo{
						Name:      child.Name,
						Kind:      "var",
						URI:       child.Position.URI,
						Line:      child.Position.Line,
						Character: child.Position.Character,
					}
				}
			}

		case "Struct":
			if node.Name != "" {
				symbols[posKey] = &ModifiedDefinitionInfo{
					Name:      node.Name,
					Kind:      "struct",
					URI:       node.Position.URI,
					Line:      node.Position.Line,
					Character: node.Position.Character,
					PackageName:  packageName,
				}
			}
			for _, field := range node.Children {
				if field.Type == "Field" {
					processField(field, "field", symbols)
				}
			}

		case "Interface":
			if node.Name != "" {
				symbols[posKey] = &ModifiedDefinitionInfo{
					Name:      node.Name,
					Kind:      "interface",
					URI:       node.Position.URI,
					Line:      node.Position.Line,
					Character: node.Position.Character,
					PackageName:  packageName,
				}
			}
			for _, method := range node.Children {
				if method.Type == "Field" {
					processField(method, "method", symbols)
				}
			}

		case "Type":
			if node.Name != "" {
				symbols[posKey] = &ModifiedDefinitionInfo{
					Name:      node.Name,
					Kind:      "type",
					URI:       node.Position.URI,
					Line:      node.Position.Line,
					Character: node.Position.Character,
					PackageName:  packageName,
				}
			}
		}

		for _, child := range node.Children {
			walk(child, node.Type)
		}
	}

	walk(ast, "")
	return symbols
}

func processField(field *SimplifiedASTNode, kind string, symbols map[string]*ModifiedDefinitionInfo) {
	var paramType string
	seenName := false

	for _, child := range field.Children {
		if child.Type == "Ident" {
			if !seenName {
				// First ident is name
				seenName = true
				nameKey := fmt.Sprintf("%s:%d:%d", child.Position.URI, child.Position.Line, child.Position.Character)
				symbols[nameKey] = &ModifiedDefinitionInfo{
					Name:      child.Name,
					Kind:      kind,
					URI:       child.Position.URI,
					Line:      child.Position.Line,
					Character: child.Position.Character,
					Type:      "", // type added later if found
				}
			} else {
				// Second ident is type
				paramType = child.Name
				// Update the last added entry with the type
				for k, v := range symbols {
					if v.Kind == kind && v.Type == "" {
						v.Type = paramType
						symbols[k] = v
						break
					}
				}
			}
		}
	}
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

func SaveSimplifiedAST(ast *SimplifiedASTNode, projectRoot, outputDir string) error {
	if ast == nil || ast.Position == nil {
		return fmt.Errorf("invalid AST node")
	}

	// Get the absolute path from URI
	uri := ast.Position.URI
	if !strings.HasPrefix(uri, "file://") {
		return fmt.Errorf("invalid URI: %s", uri)
	}
	absPath := filepath.FromSlash(strings.TrimPrefix(uri, "file://"))

	absPath, err := filepath.Abs(absPath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Derive relative path from project root
	relPath, err := filepath.Rel(projectRoot, absPath)
	if err != nil {
		return fmt.Errorf("cannot get relative path from %s to %s: %w", projectRoot, absPath, err)
	}

	// Construct output path: replace extension with .simplified.json
	jsonFileName := relPath[:len(relPath)-len(filepath.Ext(relPath))] + ".simplified.json"
	outputPath := filepath.Join(outputDir, filepath.ToSlash(jsonFileName))

	// Ensure the parent directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), os.ModePerm); err != nil {
		return err
	}

	// Write JSON file
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(ast)
}
