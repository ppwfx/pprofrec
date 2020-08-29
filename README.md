# pprofrec

Provides a single pane of glass across all runtime metrics
by recording `pprof` lookups, `runtime.MemStats` and `gopsutil` metrics
to help understand the runtime behavior of an application

[Demo](https://pprofrec-example-slzntuj6pq-uc.a.run.app/debug/pprof/window)
- Refresh to update the window.
- Scroll on the x-axis to see more features and on the y-axis to see more data points.

## motivation

Be able to instrument, inspect and troubleshoot an application in an idiomatic, fast and boring way

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
