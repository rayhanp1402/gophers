package extractor

import (
	"strings"
)

func toNodeID(path string) string {
	clean := strings.TrimSuffix(strings.ReplaceAll(path, "\\", "."), ".go")
	return strings.TrimLeft(clean, ".")
}

func isPrimitiveType(name string) bool {
	switch name {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64",
		"complex64", "complex128",
		"byte", "rune",
		"bool", "string", "error":
		return true
	default:
		return false
	}
}