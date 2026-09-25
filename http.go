package graphqlsse

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"time"
)

// ContextGenerator derives the execution context for an HTTP request.
type ContextGenerator func(context.Context, *http.Request) context.Context

// Option configures a Handler.
type Option func(*Handler)

// Handler serves GraphQL-over-SSE requests and delegates other requests.
type Handler struct {
	next                 http.Handler
	readLimit            int64
	writeTimeout         time.Duration
	heartbeatInterval    time.Duration
	maxConcurrent        int
	maxOperationDuration time.Duration
	contextGenerator     ContextGenerator
	checkOrigin          func(*http.Request) bool
	logger               *log.Logger
	semaphore            chan struct{}
	subscriber           Subscriber
}

// NewHandler creates a GraphQL-over-SSE handler.
func NewHandler(subscriber Subscriber, next http.Handler, options ...Option) *Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	h := &Handler{
		next:              next,
		readLimit:         1 << 20, // 1 MB
		writeTimeout:      15 * time.Second,
		heartbeatInterval: 15 * time.Second,
		maxConcurrent:     100,
		contextGenerator: func(ctx context.Context, _ *http.Request) context.Context {
			return ctx
		},
		checkOrigin: sameOrigin,
		logger:      log.Default(),
	}
	h.subscriber = subscriber
	for _, option := range options {
		option(h)
	}
	if h.maxConcurrent > 0 {
		h.semaphore = make(chan struct{}, h.maxConcurrent)
	}
	return h
}

// NewHandlerFunc creates a GraphQL-over-SSE handler function.
func NewHandlerFunc(sub Subscriber, next http.Handler, opts ...Option) http.HandlerFunc {
	return NewHandler(sub, next, opts...).ServeHTTP
}

// WithReadLimit limits the size of a POST request body.
func WithReadLimit(n int64) Option {
	return func(h *Handler) {
		if n > 0 {
			h.readLimit = n
		}
	}
}

// WithWriteTimeout sets the deadline applied to each SSE write.
func WithWriteTimeout(d time.Duration) Option {
	return func(h *Handler) {
		h.writeTimeout = d
	}
}

// WithHeartbeatInterval sets the interval between SSE heartbeat comments.
func WithHeartbeatInterval(interval time.Duration) Option {
	return func(h *Handler) {
		h.heartbeatInterval = interval
	}
}

// WithMaxConcurrentOperations limits simultaneously open SSE streams.
func WithMaxConcurrentOperations(n int) Option {
	return func(h *Handler) {
		h.maxConcurrent = n
	}
}

// WithContextGenerator sets the function used to derive execution contexts.
func WithContextGenerator(gen ContextGenerator) Option {
	return func(h *Handler) {
		if gen != nil {
			h.contextGenerator = gen
		}
	}
}

// WithCheckOrigin sets the origin policy for SSE requests.
func WithCheckOrigin(check func(*http.Request) bool) Option {
	return func(h *Handler) {
		if check != nil {
			h.checkOrigin = check
		}
	}
}

// WithMaxOperationDuration sets a maximum duration for each operation.
func WithMaxOperationDuration(d time.Duration) Option {
	return func(h *Handler) {
		h.maxOperationDuration = d
	}
}

// WithLogger sets the logger used for recovered panics.
func WithLogger(l *log.Logger) Option {
	return func(h *Handler) {
		if l != nil {
			h.logger = l
		}
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !acceptsEventStream(r) {
		h.next.ServeHTTP(w, r)
		return
	}
	if !h.checkOrigin(r) {
		writeHTTPError(w, http.StatusForbidden, errOriginDenied)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeHTTPError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
		return
	}
	if h.semaphore != nil {
		select {
		case h.semaphore <- struct{}{}:
			defer func() { <-h.semaphore }()
		default:
			writeHTTPError(w, http.StatusTooManyRequests, errTooManyOperations)
			return
		}
	}
	h.serveOperation(w, r)
}

func (h *Handler) acquireSubscriberContext(r *http.Request) (context.Context, context.CancelFunc) {
	ctx := h.contextGenerator(r.Context(), r)
	if h.maxOperationDuration > 0 {
		return context.WithTimeout(ctx, h.maxOperationDuration)
	}
	return ctx, func() {}
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host != r.Host {
		return false
	}
	// Reject an Origin that downgrades from HTTPS to HTTP on the same host;
	// such a mismatch never occurs for a legitimate same-origin request and
	// can indicate a cross-scheme/mixed-content attack. This check only
	// applies when TLS is terminated by this process (r.TLS != nil).
	// Deployments behind a TLS-terminating reverse proxy see r.TLS == nil
	// here and should supply a WithCheckOrigin callback that consults a
	// trusted forwarded-proto header instead.
	if r.TLS != nil && parsed.Scheme != "https" {
		return false
	}
	return true
}
