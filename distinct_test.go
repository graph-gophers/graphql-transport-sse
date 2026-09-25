package graphqlsse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"
)

func TestStreamWriteErrors(t *testing.T) {
	tests := []struct {
		name  string
		h     *Handler
		sleep time.Duration // fake-clock advance needed to fire a pending heartbeat ticker
	}{
		{
			name: "heartbeat write error",
			h: NewHandler(testSubscriber{subscribe: func(context.Context, string, string, map[string]any) (<-chan any, error) {
				return make(chan any), nil
			}}, nil, WithHeartbeatInterval(time.Millisecond)),
			sleep: 2 * time.Millisecond,
		},
		{
			name: "event write error",
			h: NewHandler(testSubscriber{subscribe: func(context.Context, string, string, map[string]any) (<-chan any, error) {
				res := make(chan any, 1)
				res <- map[string]any{"data": 1}
				return res, nil
			}}, nil, WithHeartbeatInterval(0)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				w, err := newFrameWriter(&failWriter{header: http.Header{}}, 0)
				if err != nil {
					t.Fatal(err)
				}
				done := make(chan struct{})
				go func() {
					tt.h.stream(context.Background(), w, operationRequest{Query: "{field}"})
					close(done)
				}()
				synctest.Wait()
				if tt.sleep > 0 {
					// Deterministically advances the bubble's fake clock so a
					// pending heartbeat ticker fires; this is instant in wall
					// time, unlike a real time.Sleep outside a bubble.
					time.Sleep(tt.sleep)
					synctest.Wait()
				}
				select {
				case <-done:
				default:
					t.Fatal("stream did not return after write error")
				}
			})
		})
	}
}

func TestIsBodyTooLarge(t *testing.T) {
	tests := []struct {
		name          string
		contentLength int64
		limit         int64
		want          bool
	}{
		{name: "no limit", contentLength: 100, limit: 0},
		{name: "under limit", contentLength: 10, limit: 20},
		{name: "at limit", contentLength: 20, limit: 20},
		{name: "over limit", contentLength: 21, limit: 20, want: true},
		{name: "unknown length", contentLength: -1, limit: 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.ContentLength = tt.contentLength
			if got := isBodyTooLarge(req, tt.limit); got != tt.want {
				t.Fatalf("isBodyTooLarge() = %v, want %v", got, tt.want)
			}
		})
	}
}
