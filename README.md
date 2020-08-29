[![GoDoc](https://godoc.org/github.com/ppwfx/pprofrec?status.svg)](https://godoc.org/github.com/ppwfx/pprofrec)
[![Go Report Card](https://goreportcard.com/badge/github.com/ppwfx/pprofrec)](https://goreportcard.com/report/github.com/ppwfx/pprofrec)

# pprofrec

Provides a single pane of glass across all runtime metrics
by recording `pprof` lookups, `runtime.MemStats` and `gopsutil` metrics, 
and exposing them via http endpoints to instrument, inspect and troubleshoot an application in an idiomatic, fast and boring way.

[Demo](https://pprofrec-example-slzntuj6pq-uc.a.run.app/debug/pprof/window)
- Refresh to update the window.
- Scroll on the x-axis to see more features and on the y-axis to see more data points.

## usage

Record runtime metrics at a given frequency within a given window.

```golang
windowOpts := pprofrec.WindowOpts{
    Window:    120 * time.Second,
    Frequency: 1 * time.Second,
}
mux.HandleFunc("/debug/pprof/window", pprofrec.Window(ctx, windowOpts))
```

Stream runtime metrics at a given frequency.

```golang
streamOpts := pprofrec.StreamOpts{
    Frequency: 500 * time.Millisecond,
}
mux.HandleFunc("/debug/pprof/stream", pprofrec.Stream(streamOpts))
```

Full example

```golang
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
```
