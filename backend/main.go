package main

import (
	"log"
	"net/http"
	"os"
	"time"
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
	config, err := loadConfig("config.yaml")
	if err != nil {
		log.Fatal(err)
	}
	ssh, err := loadSSHConnectionConfig(config.SSH)
	if err != nil {
		log.Fatalf("invalid SSH configuration: %v", err)
	}
	defer ssh.close()
	if config.SessionTTLHours <= 0 {
		config.SessionTTLHours = 24
	}
	app := &app{users: users, allowed: config.allowedUsers(), consolePermissions: config.consolePermissions(), ssh: ssh, sessionTTL: time.Duration(config.SessionTTLHours) * time.Hour}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/users", users.createUserHandler)
	mux.HandleFunc("POST /api/login", app.loginHandler)
	mux.HandleFunc("POST /api/logout", app.logoutHandler)
	mux.HandleFunc("GET /api/me", app.currentUserHandler)
	mux.HandleFunc("PUT /api/account", app.updateAccountHandler)
	mux.HandleFunc("POST /api/profile-image", app.uploadProfileImageHandler)
	mux.HandleFunc("GET /api/profile-image", app.profileImageHandler)
	mux.HandleFunc("POST /api/console/sessions", app.consoleSessionsHandler)
	mux.HandleFunc("POST /api/console/sessions/rename", app.renameConsoleSessionHandler)
	mux.HandleFunc("GET /api/console/terminal", app.consoleTerminalHandler)
	mux.HandleFunc("GET /settings.html", app.settingsHandler)
	mux.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir("frontend/css"))))
	mux.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir("frontend/js"))))
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("frontend/assets"))))
	mux.HandleFunc("/", app.rootHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	address := "127.0.0.1:" + port
	log.Printf("Serving locally at http://%s", address)
	log.Fatal(http.ListenAndServe(address, mux))
}
