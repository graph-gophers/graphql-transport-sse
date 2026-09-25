package graphqlsse

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type frameWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	timeout time.Duration
}

func newFrameWriter(w http.ResponseWriter, timeout time.Duration) (*frameWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("response writer does not support flushing")
	}
	return &frameWriter{w: w, flusher: flusher, timeout: timeout}, nil
}

func (w *frameWriter) writeEvent(event string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return w.write("event: " + event + "\ndata: " + string(data) + "\n\n")
}

func (w *frameWriter) writeComplete() error {
	return w.write("event: complete\ndata: \n\n")
}

func (w *frameWriter) heartbeat() error {
	return w.write(": heartbeat\n\n")
}

func (w *frameWriter) write(value string) error {
	if w.timeout > 0 {
		if err := http.NewResponseController(w.w).SetWriteDeadline(time.Now().Add(w.timeout)); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w.w, value); err != nil {
		return err
	}
	w.flusher.Flush()
	return nil
}
