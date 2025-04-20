package extractor

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
)

// Parses a whole package (only the .go files) into a FileSet
// dir is relative to this (gophers) package
func ParsePackage(dir string) (*token.FileSet, map[string]*ast.File, error) {
    fset := token.NewFileSet()

    files := make(map[string]*ast.File)

    err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
        if filepath.Ext(path) == ".go" {
            file, err := os.Open(path)
            if err != nil {
                return err
            }
            defer file.Close()

            astFile, err := parser.ParseFile(fset, path, file, parser.AllErrors)
            if err != nil {
                return err
            }
    
            files[path] = astFile
        }

        return nil
    })

    if err != nil {
        return nil, nil, err
    }

    return fset, files, nil
}