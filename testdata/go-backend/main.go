package main

import (
	"log"
	"net/http"

	"example.com/go-backend/handlers"
)

type UselessInt int

func main() {
	http.HandleFunc("/calculate", handlers.CalculateHandler)

	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "" {
			name = handlers.DefaultName
		}
		handlers.SayHello(w, name)
	})

	log.Println("Server running on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
