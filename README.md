# pprofrec

Provides a single pane of glass across all runtime metrics of a go process
by recording `pprof` lookups, `runtime.MemStats` and `gopsutil` metrics
to help understand the runtime behavior of an application.

## Usage

Records runtime metrics at a given frequency within a given window

```golang
windowOpts := pprofrec.WindowOpts{
    Window:    120 * time.Second,
    Frequency: 1 * time.Second,
}
mux.HandleFunc("/debug/pprof/window", pprofrec.Window(ctx, windowOpts))
```

Streams runtime metrics at a given frequency

```golang
streamOpts := pprofrec.StreamOpts{
    Frequency: 500 * time.Millisecond,
}
mux.HandleFunc("/debug/pprof/stream", pprofrec.Stream(streamOpts))
```

