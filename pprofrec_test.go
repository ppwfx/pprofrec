package pprofrec

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStream(t *testing.T) {
	f := Stream(StreamOpts{Frequency: 100 * time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:8080", nil)
	require.NoError(t, err)

	w := &responseWriter{}
	go func() {
		f(w, r)
	}()

	time.Sleep(500 * time.Millisecond)
	cancel()

	assert.Contains(t, w.Buffer.String(), "MiB")
}

type responseWriter struct {
	Buffer     bytes.Buffer
	StatusCode int
}

func (w *responseWriter) Header() http.Header {
	return http.Header{}
}

func (w *responseWriter) Write(b []byte) (int, error) {
	return w.Buffer.Write(b)
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.StatusCode = statusCode
}

func (w *responseWriter) Flush() {}
