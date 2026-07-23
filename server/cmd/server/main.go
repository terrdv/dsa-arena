package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/terrdv/dsa-arena/server/internal/database"
	"github.com/terrdv/dsa-arena/server/internal/matchmaking"
	"github.com/terrdv/dsa-arena/server/internal/handlers"
	"github.com/terrdv/dsa-arena/server/internal/cache"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// initiate database connection
	db, err := database.Connect(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer db.Close()

	// redis cache
	redis, err := cache.Connect(context.Background(), os.Getenv("REDIS_ADDR"))
	if err != nil {
		log.Fatalf("connect redis: %v", err)
	}
	defer redis.Close()

	// global queue
	queue := matchmaking.NewQueue()

	// room state store
	rooms := matchmaking.NewRoomStore(redis)

	go matchmaking.RunMatcher(context.Background(), queue, rooms)

	//initiate multiplexer 
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /queue", handlers.JoinQueue(queue))

	log.Printf("listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
