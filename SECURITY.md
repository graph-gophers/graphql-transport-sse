# Security policy

Report security issues privately to the repository maintainers rather than opening a public issue. Include the affected version, reproduction steps, and impact.

## Deployment requirements

This package is a transport layer; the embedding application must:

- Authenticate and authorize requests. Use `WithContextGenerator` and the underlying GraphQL implementation to apply identity and access checks.
- Serve SSE over HTTPS and protect authentication cookies with `Secure`, `HttpOnly`, and an appropriate `SameSite` value.
- Configure `WithMaxConcurrentOperations` and `WithMaxOperationDuration`, plus per-client rate and connection limits at the reverse proxy or edge. The built-in concurrency limit is global to one handler, not per client.
- Configure proxy/server request, idle, read, and write timeouts, and disable response buffering for the SSE endpoint.
- Ensure `Subscriber` implementations honor context cancellation and close result channels.

## Important considerations

- The default origin check allows requests without an `Origin` header and compares the origin host with the request host. Behind a TLS-terminating proxy, or when a strict allowlist is required, provide a scheme-aware `WithCheckOrigin` callback. Do not use permissive or substring-based checks.
- GET subscriptions put the query, operation name, and variables in the URL. They may be recorded in browser history, access logs, or `Referer` headers; never include credentials or other secrets in them.
- Errors returned by `Subscriber.Subscribe` are sent to the client. Return client-safe messages and avoid exposing backend or internal error details.
- `WithReadLimit` limits request size but does not prevent slow uploads; use server or proxy read timeouts as well.
