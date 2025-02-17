package extractor

import (
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/tools/go/packages"
)

const outputDir = "./out"

func ParsePackage(pkgPath string) {
	// Resolve absolute path
	cfg := &packages.Config{
		Mode:  packages.LoadSyntax,
		Fset:  token.NewFileSet(),
		Tests: false,
		Dir:   pkgPath,
	}

	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil {
		log.Fatalf("Failed to load package: %v", err)
	}

	// Ensure pkgPath doesn't end with a separator (e.g., backslash or forward slash)
	cleanPkgPath := filepath.Base(pkgPath) // Get the base name of the package directory
	outputFilePath := filepath.Join(outputDir, cleanPkgPath+"_ast.txt")

	outFile, err := os.Create(outputFilePath)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer outFile.Close()

	// Set the output destination for log
	log.SetOutput(outFile)

	// Traverse the ASTs for all files in the package
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			fmt.Fprintf(outFile, "AST for file: %s\n", pkg.Fset.Position(file.Pos()))
			ast.Fprint(outFile, pkg.Fset, file, nil)  // Write the AST to file
			fmt.Fprintln(outFile)  // Newline for better readability
		}
	}

	log.Printf("AST has been written to %s", outputFilePath)
}
