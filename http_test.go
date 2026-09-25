package graphqlsse

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

type testSubscriber struct {
	subscribe func(context.Context, string, string, map[string]any) (<-chan any, error)
}

func (s testSubscriber) Subscribe(ctx context.Context, query, operationName string, variables map[string]any) (<-chan any, error) {
	return s.subscribe(ctx, query, operationName, variables)
}

func TestHandler(t *testing.T) {
	tests := []struct {
		name       string
		req        func(*testing.T, string) *http.Request
		opts       []Option
		sub        testSubscriber
		wantStatus int
		wantBody   string
	}{
		{
			name: "stream",
			req: func(_ *testing.T, url string) *http.Request {
				req, _ := http.NewRequest(http.MethodGet, url+"?query=%7Bfield%7D", nil)
				req.Header.Set("Accept", "text/event-stream")
				return req
			},
			sub: testSubscriber{subscribe: func(context.Context, string, string, map[string]any) (<-chan any, error) {
				res := make(chan any, 2)
				res <- map[string]any{"data": map[string]string{"field": "one"}}
				res <- map[string]any{"data": map[string]string{"field": "two"}}
				close(res)
				return res, nil
			}},
			wantBody: "event: next\ndata: {\"data\":{\"field\":\"one\"}}\n\nevent: next\ndata: {\"data\":{\"field\":\"two\"}}\n\nevent: complete\ndata: \n\n",
		},
		{
			name: "fallback",
			req: func(_ *testing.T, url string) *http.Request {
				req, _ := http.NewRequest(http.MethodGet, url, nil)
				return req
			},
			sub: testSubscriber{subscribe: func(context.Context, string, string, map[string]any) (<-chan any, error) {
				panic("must not be called")
			}},
			wantBody: "fallback",
		},
		{
			name: "origin denied",
			req: func(_ *testing.T, url string) *http.Request {
				req, _ := http.NewRequest(http.MethodGet, url+"?query=%7Bfield%7D", nil)
				req.Header.Set("Accept", "text/event-stream")
				req.Header.Set("Origin", "https://other.example")
				return req
			},
			sub: testSubscriber{subscribe: func(context.Context, string, string, map[string]any) (<-chan any, error) {
				panic("must not be called")
			}},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "concurrency limit",
			req: func(_ *testing.T, url string) *http.Request {
				req, _ := http.NewRequest(http.MethodGet, url+"?query=%7Bfield%7D", nil)
				req.Header.Set("Accept", "text/event-stream")
				return req
			},
			opts: []Option{WithMaxConcurrentOperations(0)},
			sub: testSubscriber{subscribe: func(context.Context, string, string, map[string]any) (<-chan any, error) {
				res := make(chan any)
				close(res)
				return res, nil
			}},
			wantStatus: http.StatusOK,
		},
		{
			name: "bad request",
			req: func(_ *testing.T, url string) *http.Request {
				req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(`{"query":"{ field }"}`))
				req.Header.Set("Accept", "text/event-stream")
				req.Header.Set("Content-Type", "text/plain")
				return req
			},
			sub: testSubscriber{subscribe: func(context.Context, string, string, map[string]any) (<-chan any, error) {
				panic("must not be called")
			}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "method not allowed",
			req: func(_ *testing.T, url string) *http.Request {
				req, _ := http.NewRequest(http.MethodPut, url+"?query=%7Bfield%7D", nil)
				req.Header.Set("Accept", "text/event-stream")
				return req
			},
			sub: testSubscriber{subscribe: func(context.Context, string, string, map[string]any) (<-chan any, error) {
				panic("must not be called")
			}},
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name: "request too large",
			req: func(_ *testing.T, url string) *http.Request {
				req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(`{"query":"{ field }"}`))
				req.Header.Set("Accept", "text/event-stream")
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			opts: []Option{WithReadLimit(5)},
			sub: testSubscriber{subscribe: func(context.Context, string, string, map[string]any) (<-chan any, error) {
				panic("must not be called")
			}},
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name: "subscribe error",
			req: func(_ *testing.T, url string) *http.Request {
				req, _ := http.NewRequest(http.MethodGet, url+"?query=%7Bfield%7D", nil)
				req.Header.Set("Accept", "text/event-stream")
				return req
			},
			sub: testSubscriber{subscribe: func(context.Context, string, string, map[string]any) (<-chan any, error) {
				return nil, errors.New("boom")
			}},
			wantBody: "event: next\ndata: {\"errors\":[{\"message\":\"boom\"}]}\n\nevent: complete\ndata: \n\n",
		},
		{
			name: "panic recovery",
			req: func(_ *testing.T, url string) *http.Request {
				req, _ := http.NewRequest(http.MethodGet, url+"?query=%7Bfield%7D", nil)
				req.Header.Set("Accept", "text/event-stream")
				return req
			},
			sub: testSubscriber{subscribe: func(context.Context, string, string, map[string]any) (<-chan any, error) {
				panic("boom")
			}},
			wantBody: "event: next\ndata: {\"errors\":[{\"message\":\"internal server error\"}]}\n\nevent: complete\ndata: \n\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fallback := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, "fallback")
			})
			opts := append(tt.opts, WithHeartbeatInterval(0), WithWriteTimeout(time.Second), WithLogger(log.New(io.Discard, "", 0)))
			h := NewHandler(tt.sub, fallback, opts...)
			srv := httptest.NewServer(h)
			defer srv.Close()
			resp, err := srv.Client().Do(tt.req(t, srv.URL))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			wantStatus := tt.wantStatus
			if wantStatus == 0 {
				wantStatus = http.StatusOK
			}
			if resp.StatusCode != wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", resp.StatusCode, wantStatus, body)
			}
			if tt.wantBody != "" && string(body) != tt.wantBody {
				t.Fatalf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

type ctxKey struct{}

func TestOptions(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		want func(*testing.T, *Handler)
	}{
		{
			name: "read limit",
			opts: []Option{WithReadLimit(42)},
			want: func(t *testing.T, h *Handler) {
				if h.readLimit != 42 {
					t.Fatalf("readLimit = %d, want 42", h.readLimit)
				}
			},
		},
		{
			name: "read limit ignores non-positive",
			opts: []Option{WithReadLimit(0)},
			want: func(t *testing.T, h *Handler) {
				if h.readLimit != 1<<20 {
					t.Fatalf("readLimit = %d, want default", h.readLimit)
				}
			},
		},
		{
			name: "write timeout",
			opts: []Option{WithWriteTimeout(2 * time.Second)},
			want: func(t *testing.T, h *Handler) {
				if h.writeTimeout != 2*time.Second {
					t.Fatalf("writeTimeout = %v, want 2s", h.writeTimeout)
				}
			},
		},
		{
			name: "max operation duration",
			opts: []Option{WithMaxOperationDuration(3 * time.Second)},
			want: func(t *testing.T, h *Handler) {
				if h.maxOperationDuration != 3*time.Second {
					t.Fatalf("maxOperationDuration = %v, want 3s", h.maxOperationDuration)
				}
			},
		},
		{
			name: "context generator",
			opts: []Option{WithContextGenerator(func(ctx context.Context, _ *http.Request) context.Context {
				return context.WithValue(ctx, ctxKey{}, "v")
			})},
			want: func(t *testing.T, h *Handler) {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				ctx := h.contextGenerator(context.Background(), req)
				if ctx.Value(ctxKey{}) != "v" {
					t.Fatal("context generator was not applied")
				}
			},
		},
		{
			name: "context generator ignores nil",
			opts: []Option{WithContextGenerator(nil)},
			want: func(t *testing.T, h *Handler) {
				if h.contextGenerator == nil {
					t.Fatal("contextGenerator should keep default when nil")
				}
			},
		},
		{
			name: "check origin",
			opts: []Option{WithCheckOrigin(func(*http.Request) bool { return false })},
			want: func(t *testing.T, h *Handler) {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				if h.checkOrigin(req) {
					t.Fatal("checkOrigin override was not applied")
				}
			},
		},
		{
			name: "check origin ignores nil",
			opts: []Option{WithCheckOrigin(nil)},
			want: func(t *testing.T, h *Handler) {
				if h.checkOrigin == nil {
					t.Fatal("checkOrigin should keep default when nil")
				}
			},
		},
		{
			name: "logger",
			opts: []Option{WithLogger(log.New(io.Discard, "x", 0))},
			want: func(t *testing.T, h *Handler) {
				if h.logger == nil {
					t.Fatal("logger was not applied")
				}
			},
		},
		{
			name: "logger ignores nil",
			opts: []Option{WithLogger(nil)},
			want: func(t *testing.T, h *Handler) {
				if h.logger == nil {
					t.Fatal("logger should keep default when nil")
				}
			},
		},
		{
			name: "max concurrent operations disables semaphore",
			opts: []Option{WithMaxConcurrentOperations(0)},
			want: func(t *testing.T, h *Handler) {
				if h.semaphore != nil {
					t.Fatal("semaphore should be nil when max concurrent is 0")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(testSubscriber{}, nil, tt.opts...)
			tt.want(t, h)
		})
	}
}

func TestAcquireSubscriberContext(t *testing.T) {
	tests := []struct {
		name         string
		opts         []Option
		wantDeadline bool
	}{
		{name: "no max duration", opts: nil, wantDeadline: false},
		{name: "with max duration", opts: []Option{WithMaxOperationDuration(time.Minute)}, wantDeadline: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(testSubscriber{}, nil, tt.opts...)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			ctx, cancel := h.acquireSubscriberContext(req)
			defer cancel()
			_, ok := ctx.Deadline()
			if ok != tt.wantDeadline {
				t.Fatalf("deadline set = %v, want %v", ok, tt.wantDeadline)
			}
		})
	}
}

func TestSameOrigin(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		host   string
		tls    bool
		want   bool
	}{
		{name: "no origin header", origin: "", host: "example.com", want: true},
		{name: "matching origin", origin: "https://example.com", host: "example.com", want: true},
		{name: "different origin", origin: "https://other.example", host: "example.com", want: false},
		{name: "malformed origin", origin: "http://[::1", host: "example.com", want: false},
		{name: "http origin without tls", origin: "http://example.com", host: "example.com", want: true},
		{name: "http origin with tls downgrade", origin: "http://example.com", host: "example.com", tls: true, want: false},
		{name: "https origin with tls", origin: "https://example.com", host: "example.com", tls: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, "/", nil)
			req.Host = tt.host
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.tls {
				req.TLS = &tls.ConnectionState{}
			}
			if got := sameOrigin(req); got != tt.want {
				t.Fatalf("sameOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandlerCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		canceled := make(chan struct{})
		sub := testSubscriber{subscribe: func(ctx context.Context, _, _ string, _ map[string]any) (<-chan any, error) {
			res := make(chan any)
			go func() {
				<-ctx.Done()
				close(canceled)
				close(res)
			}()
			return res, nil
		}}
		h := NewHandler(sub, nil, WithHeartbeatInterval(0))
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/?query=%7Bfield%7D", nil)
		req.Header.Set("Accept", "text/event-stream")
		done := make(chan struct{})
		go func() {
			h.ServeHTTP(httptest.NewRecorder(), req)
			close(done)
		}()
		synctest.Wait()
		cancel()
		synctest.Wait()
		select {
		case <-canceled:
		default:
			t.Fatal("subscription context was not canceled")
		}
		<-done
	})
}

func TestHandlerHeartbeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sub := testSubscriber{subscribe: func(context.Context, string, string, map[string]any) (<-chan any, error) {
			return make(chan any), nil
		}}
		h := NewHandler(sub, nil, WithHeartbeatInterval(time.Second), WithWriteTimeout(0))
		ctx, cancel := context.WithCancel(t.Context())
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/?query=%7Bfield%7D", nil)
		req.Header.Set("Accept", "text/event-stream")
		rec := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			h.ServeHTTP(rec, req)
			close(done)
		}()
		synctest.Wait()
		time.Sleep(3 * time.Second)
		synctest.Wait()
		cancel()
		synctest.Wait()
		<-done
		if got := strings.Count(rec.Body.String(), ": heartbeat\n\n"); got < 2 {
			t.Fatalf("got %d heartbeats, want at least 2; body = %q", got, rec.Body.String())
		}
	})
}

func TestServeOperationNoFlusher(t *testing.T) {
	h := NewHandler(testSubscriber{subscribe: func(context.Context, string, string, map[string]any) (<-chan any, error) {
		panic("must not be called")
	}}, nil)
	req, _ := http.NewRequest(http.MethodGet, "/?query=%7Bfield%7D", nil)
	req.Header.Set("Accept", "text/event-stream")
	inner := httptest.NewRecorder()
	h.ServeHTTP(struct{ http.ResponseWriter }{inner}, req)
	if inner.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", inner.Code, http.StatusInternalServerError)
	}
}

func TestServeHTTPTooManyConcurrent(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	sub := testSubscriber{subscribe: func(context.Context, string, string, map[string]any) (<-chan any, error) {
		close(started)
		<-release
		res := make(chan any)
		close(res)
		return res, nil
	}}
	h := NewHandler(sub, nil, WithHeartbeatInterval(0), WithMaxConcurrentOperations(1))
	srv := httptest.NewServer(h)
	defer srv.Close()

	first := make(chan *http.Response, 1)
	go func() {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"?query=%7Bfield%7D", nil)
		req.Header.Set("Accept", "text/event-stream")
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Error(err)
			return
		}
		first <- resp
	}()

	<-started
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"?query=%7Bfield%7D", nil)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusTooManyRequests)
	}

	close(release)
	resp1 := <-first
	defer func() { _ = resp1.Body.Close() }()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first status = %d, want %d", resp1.StatusCode, http.StatusOK)
	}
}

func TestAcceptsEventStream(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	for _, accept := range []string{"text/event-stream", "application/json, text/event-stream; q=1"} {
		req.Header.Set("Accept", accept)
		if !acceptsEventStream(req) {
			t.Fatalf("accept %q was rejected", accept)
		}
	}
	for _, accept := range []string{"application/json", "text/html,application/xhtml+xml,*/*;q=0.8"} {
		req.Header.Set("Accept", strings.TrimSpace(accept))
		if acceptsEventStream(req) {
			t.Fatalf("accept %q was wrongly accepted", accept)
		}
	}
}
