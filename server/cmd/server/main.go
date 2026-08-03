package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/terrdv/dsa-arena/server/internal/cache"
	"github.com/terrdv/dsa-arena/server/internal/database"
	"github.com/terrdv/dsa-arena/server/internal/handlers"
	"github.com/terrdv/dsa-arena/server/internal/matchmaking"
	"github.com/terrdv/dsa-arena/server/internal/workers"
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
	hub := matchmaking.NewHub()

	// judge worker pool: bounds how many judge containers run at once
	pool := workers.NewWorkerPool(8)
	pool.Start()

	go matchmaking.RunMatcher(context.Background(), queue, rooms, db)

	//initiate multiplexer
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /queue", handlers.JoinQueue(queue))

	mux.HandleFunc("GET /room/{match_id}", handlers.GetRoom(rooms))
	mux.HandleFunc("GET /room/{match_id}/session", handlers.RoomSession(rooms, hub, pool))

	log.Printf("listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
