package main

import (
	"fmt"
	"os"

	"kiro/server"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	server.StartTokenRefresher()

	port := os.Getenv("PORT")
	if port == "" {
		port = "1188"
	}

	fmt.Printf("Kiro2API Proxy Server starting on port %s\n", port)
	server.StartServer(port)
}
