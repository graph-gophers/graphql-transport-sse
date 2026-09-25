package main

import (
	"context"
	_ "embed"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"

	graphqlsse "github.com/graph-gophers/graphql-transport-sse"
)

var (
	//go:embed index.html
	graphiqlHTML []byte

	//go:embed schema.graphql
	schemaSDL string
)

type resolver struct {
	count atomic.Int32
}

// newResolver starts the single background goroutine that owns the counter,
// incrementing it once per second regardless of how many subscribers are
// listening.
func newResolver(ctx context.Context) *resolver {
	r := &resolver{}
	go r.tick(ctx)
	return r
}

func (r *resolver) tick(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		select {
		case <-ctx.Done():
			return
		default:
			r.count.Add(1)
		}
	}
}

type queryResolver struct {
	r *resolver
}

type subscriptionResolver struct {
	r *resolver
}

func (r *resolver) Query() *queryResolver               { return &queryResolver{r} }
func (r *resolver) Subscription() *subscriptionResolver { return &subscriptionResolver{r} }

func (q *queryResolver) Counter() int32 { return q.r.count.Load() }

func (s *subscriptionResolver) Counter(ctx context.Context) <-chan int32 {
	ch := make(chan int32)
	go func() {
		defer close(ch)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n := s.r.count.Load()
				select {
				case ch <- n:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return ch
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	schema := graphql.MustParseSchema(schemaSDL, newResolver(ctx), graphql.UseStringDescriptions())
	mux := http.NewServeMux()

	// GraphiQL UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, err := w.Write(graphiqlHTML)
		if err != nil {
			log.Printf("error writing GraphiQL HTML: %v", err)
		}
	})

	// GraphQL endpoint — handles both HTTP POST and SSE (graphql-transport-sse)
	mux.Handle("/graphql", graphqlsse.NewHandlerFunc(schema, &relay.Handler{Schema: schema}))

	addr := ":8080"
	log.Printf("GraphQL  -> http://localhost%s/graphql", addr)
	log.Printf("GraphiQL -> http://localhost%s/", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
