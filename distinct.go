package graphqlsse

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type constErr string

func (e constErr) Error() string { return string(e) }

const (
	errOriginDenied      = constErr("origin is not allowed")
	errMethodNotAllowed  = constErr("method must be GET or POST")
	errTooManyOperations = constErr("too many concurrent operations")
	errRequestTooLarge   = constErr("request body exceeds limit")
)

func (h *Handler) serveOperation(w http.ResponseWriter, r *http.Request) {
	operation, err := parseOperation(r, h.readLimit)
	if err != nil {
		if errors.Is(err, errRequestTooLarge) || (r.Method == http.MethodPost && isBodyTooLarge(r, h.readLimit)) {
			writeHTTPError(w, http.StatusRequestEntityTooLarge, err)
			return
		}
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	writer, err := newFrameWriter(w, h.writeTimeout)
	if err != nil {
		writeHTTPError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	writer.flusher.Flush()
	ctx, cancel := h.acquireSubscriberContext(r)
	defer cancel()
	h.stream(ctx, writer, operation)
}

func (h *Handler) stream(ctx context.Context, writer *frameWriter, operation operationRequest) {
	defer func() {
		if recovered := recover(); recovered != nil {
			h.logger.Printf("graphql SSE panic: %v", recovered)
			_ = writeStreamError(writer, fmt.Errorf("internal server error"))
		}
		_ = writer.writeComplete()
	}()
	results, err := h.subscriber.Subscribe(ctx, operation.Query, operation.OperationName, operation.Variables)
	if err != nil {
		_ = writeStreamError(writer, err)
		return
	}
	var heartbeat <-chan time.Time
	var ticker *time.Ticker
	if h.heartbeatInterval > 0 {
		ticker = time.NewTicker(h.heartbeatInterval)
		defer ticker.Stop()
		heartbeat = ticker.C
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat:
			if err := writer.heartbeat(); err != nil {
				return
			}
		case res, ok := <-results:
			if !ok {
				return
			}
			if err := writer.writeEvent("next", res); err != nil {
				return
			}
		}
	}
}

func isBodyTooLarge(r *http.Request, limit int64) bool {
	if limit <= 0 || r.ContentLength <= limit {
		return false
	}
	return true
}
