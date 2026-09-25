package graphqlsse

import "context"

// Subscriber executes a GraphQL document and returns its result stream.
type Subscriber interface {
	Subscribe(context.Context, string, string, map[string]any) (<-chan any, error)
}
