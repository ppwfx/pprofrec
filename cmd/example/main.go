package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/ppwfx/pprofrec"
)

func main() {
	mux := http.NewServeMux()

	ctx := context.Background()

	windowOpts := pprofrec.WindowOpts{
		Window:    120 * time.Second,
		Frequency: 1 * time.Second,
	}
	mux.HandleFunc("/debug/pprof/window", pprofrec.Window(ctx, windowOpts))

	streamOpts := pprofrec.StreamOpts{
		Frequency: 500 * time.Millisecond,
	}
	mux.HandleFunc("/debug/pprof/stream", pprofrec.Stream(streamOpts))

	srv := &http.Server{
		Addr:         ":8080",
		WriteTimeout: 15 * time.Minute,
		Handler:      mux,
	}

	log.Printf("listens on: %v", srv.Addr)

	err := srv.ListenAndServe()
	if err != nil {
		log.Fatalf("failed to listen: %v", err)

		return
	}
}
