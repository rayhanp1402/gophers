package extractor_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rayhanp1402/gophers/extractor"
)

func TestSimplifiedASTBuilder(t *testing.T) {
	// remember where we started, so we can return
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	// compute absolute paths up front
	inputDirRel := "../testdata/go-backend"
	expectedDirRel := "../testdata/outputs/intermediate_representation"

	inputDir, err := filepath.Abs(filepath.Join(originalWD, inputDirRel))
	if err != nil {
		t.Fatalf("failed to resolve inputDir: %v", err)
	}
	expectedDir, err := filepath.Abs(filepath.Join(originalWD, expectedDirRel))
	if err != nil {
		t.Fatalf("failed to resolve expectedDir: %v", err)
	}

	// sanity-check the expected folder exists
	if fi, err := os.Stat(expectedDir); err != nil {
		t.Fatalf("cannot stat expectedDir %q: %v", expectedDir, err)
	} else if !fi.IsDir() {
		t.Fatalf("expectedDir %q is not a directory", expectedDir)
	}

	// switch into the target project so go/packages can resolve imports
	if err := os.Chdir(inputDir); err != nil {
		t.Fatalf("chdir to %q failed: %v", inputDir, err)
	}
	defer os.Chdir(originalWD)

	// --- now run your real pipeline, starting from "."
	outputDir := t.TempDir()

	fset, parsedFiles, err := extractor.ParsePackage(".")
	if err != nil {
		t.Fatalf("ParsePackage failed: %v", err)
	}

	absPath, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("Abs path failed: %v", err)
	}

	typesInfo, _, err := extractor.LoadTypesInfo(fset, parsedFiles, absPath)
	if err != nil {
		t.Fatalf("LoadTypesInfo failed: %v", err)
	}

	if err := extractor.OutputSimplifiedASTs(fset, parsedFiles, absPath, outputDir, typesInfo); err != nil {
		t.Fatalf("OutputSimplifiedASTs failed: %v", err)
	}

	// walk the *absolute* expectedDir (not relative!)
	err = filepath.Walk(expectedDir, func(expectedPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(expectedPath) != ".json" {
			return nil
		}

		relPath, _ := filepath.Rel(expectedDir, expectedPath)
		actualPath := filepath.Join(outputDir, relPath)

		expectedBytes, _ := os.ReadFile(expectedPath)
		actualBytes, err := os.ReadFile(actualPath)
		if err != nil {
			t.Errorf("missing or unreadable output %q: %v", actualPath, err)
			return nil
		}

		var exp, act interface{}
		json.Unmarshal(expectedBytes, &exp)
		json.Unmarshal(actualBytes, &act)

		if a, _ := json.Marshal(exp); string(a) != string(jsonMustMarshal(act)) {
			t.Errorf("mismatch in %s", relPath)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("error walking expectedDir: %v", err)
	}
}

func jsonMustMarshal(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}
