// Package graphqlsse serves GraphQL operations over Server-Sent Events.
//
// The handler implements the distinct-connections mode of the GraphQL over SSE
// protocol. Each GET or POST request with an event-stream Accept header opens
// one stream and receives next events followed by complete. Non-SSE requests
// are delegated to the wrapped handler.
//
// The default origin policy accepts same-origin requests and requests without
// an Origin header. Configure WithCheckOrigin explicitly when serving browsers
// from another origin.
package graphqlsse
