package graphqlsse

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/graph-gophers/graphql-go"
)

const exampleSchema = `
	schema {
		query: Query
		subscription: Subscription
	}

	type Query {
		hello: String!
	}

	type Subscription {
		hello: String!
	}
`

type exampleResolver struct{}

type exampleQueryResolver struct{}

type exampleSubscriptionResolver struct{}

func (*exampleResolver) Query() *exampleQueryResolver {
	return &exampleQueryResolver{}
}

func (*exampleResolver) Subscription() *exampleSubscriptionResolver {
	return &exampleSubscriptionResolver{}
}

func (*exampleQueryResolver) Hello() string {
	return "world"
}

func (*exampleSubscriptionResolver) Hello(context.Context) <-chan string {
	res := make(chan string, 1)
	res <- "world"
	close(res)
	return res
}

func ExampleNewHandlerFunc() {
	schema := graphql.MustParseSchema(exampleSchema, &exampleResolver{})
	h := NewHandlerFunc(schema, nil, WithHeartbeatInterval(0))
	srv := httptest.NewServer(h)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"?query=subscription%20%7Bhello%7D", nil)
	req.Header.Set("Accept", "text/event-stream")
	res, _ := srv.Client().Do(req)
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(res.Body)
	fmt.Print(strings.TrimSpace(string(body)))

	// Output:
	// event: next
	// data: {"data":{"hello":"world"}}
	//
	// event: complete
	// data:
}
