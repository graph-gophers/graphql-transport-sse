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
them. The default check compares host and — when this process terminates TLS directly
(`r.TLS != nil`) — rejects an `Origin` that downgrades from `https` to `http` on the same
host. It does not otherwise validate scheme, so deployments behind a TLS-terminating
reverse proxy should supply a `WithCheckOrigin` callback that consults a trusted
forwarded-proto header if strict scheme checking is required. This default matters
because cookie-authenticated SSE streams can expose subscription data to an unintended
origin. Prefer SameSite cookies or bearer authentication initiated by `fetch`, and
configure an explicit origin allowlist when cross-origin access is required.

See [SECURITY.md](SECURITY.md) for a complete list of security assumptions and
recommendations for embedders.

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
