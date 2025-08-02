package handlers

import (
	"fmt"
	"net/http"
)

const DefaultName = "John Doe"

func SayHello(w http.ResponseWriter, name string) {
	fmt.Fprintf(w, "Hello, %s!\n", name)
}
