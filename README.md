# GraphQL subscriptions over SSE

`graphql-transport-sse` serves GraphQL operations over Server-Sent Events using the
distinct-connections mode of the GraphQL over SSE protocol. It accepts `GET` and `POST`
operations with `Accept: text/event-stream`, emits `next` events, and terminates each
operation with `complete`. Other requests are passed to the wrapped HTTP handler.

## Getting Started
```go
schema := graphql.MustParseSchema(schemaString, resolver)
sse := graphqlsse.NewHandler(schema, relay.Handler{Schema: schema})
http.Handle("/graphql", sse)
log.Fatal(http.ListenAndServe(":8080", nil))
```

## Example
Run the standalone counter example with:

```sh
go run ./example
```

Then open <http://localhost:8080> for a GraphiQL UI wired up to the SSE endpoint at
`/graphql` (queries/mutations use plain JSON POST responses via `relay.Handler`;
`subscription { counter }` streams live ticks over SSE).

## Security

Requests without an `Origin` header and same-origin browser requests are allowed by
default. Cross-origin requests are rejected unless `WithCheckOrigin` explicitly allows
them. 

## Options

- `WithReadLimit` bounds POST bodies and the raw GET query string.
- `WithWriteTimeout` bounds each write,
- `WithHeartbeatInterval` sends keepalive comments,
- `WithMaxConcurrentOperations` limits open streams,
- `WithMaxOperationDuration` bounds stream lifetime,
- `WithContextGenerator` injects request-derived context, and
- `WithCheckOrigin` configures browser origin checks.

## Limitations
SSE streams do not resume after reconnecting. Use application-level cursors or offsets when
resumption is required. Reverse proxies must disable response buffering for the endpoint.

The single-connection multiplexed mode is intentionally not part of v1.
