package main

import (
	"log"
	"os"

	"github.com/VitaliAndrushkevich/pulse/internal/api"
)

func main() {
	port := os.Getenv("PULSE_PORT")
	if port == "" {
		port = "8080"
	}

	r := api.NewRouter()
	addr := ":" + port
	log.Printf("pulse listening on %s", addr)

	if err := r.Run(addr); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}
