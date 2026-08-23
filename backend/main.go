package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatal(err)
	}

	users, err := openUserStore("data/users.db")
	if err != nil {
		log.Fatal(err)
	}
	defer users.close()
	allowed, err := loadAllowedUsers("config.yaml")
	if err != nil {
		log.Fatal(err)
	}
	app := &app{users: users, allowed: allowed, sessions: make(map[string]int64)}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/users", users.createUserHandler)
	mux.HandleFunc("POST /api/login", app.loginHandler)
	mux.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir("frontend/css"))))
	mux.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir("frontend/js"))))
	mux.HandleFunc("/", app.rootHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Serving http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
