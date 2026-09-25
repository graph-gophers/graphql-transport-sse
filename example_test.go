package graphqlsse

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
)

type exampleSubscriber struct{}

func (exampleSubscriber) Subscribe(context.Context, string, string, map[string]any) (<-chan any, error) {
	results := make(chan any, 1)
	results <- map[string]any{"data": map[string]string{"hello": "world"}}
	close(results)
	return results, nil
}

func ExampleNewHandlerFunc() {
	handler := NewHandlerFunc(exampleSubscriber{}, nil, WithHeartbeatInterval(0))
	server := httptest.NewServer(handler)
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL+"?query=%7Bhello%7D", nil)
	req.Header.Set("Accept", "text/event-stream")
	res, _ := server.Client().Do(req)
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
