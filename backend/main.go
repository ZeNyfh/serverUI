package main

import (
	"log"
	"net/http"
)

func main() {
	http.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir("frontend/css"))))
	http.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir("frontend/js"))))
	http.Handle("/", http.FileServer(http.Dir("frontend/html")))

	log.Println("Serving http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
