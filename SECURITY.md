# Security policy

Report security issues privately to the repository maintainers rather than opening a
public issue. Include the affected version, reproduction steps, and impact.

## Assumptions this library makes about its embedder/deployment

`graphql-transport-sse` is a transport layer. It pushes several security
responsibilities to the application embedding it:

- The embedder authenticates requests and applies authorization inside
  `ContextGenerator`, `Subscriber`, or the wrapped GraphQL implementation. The
  library itself does not authenticate, authorize, or rate-limit by identity.
- `Subscriber` implementations honor request cancellation, stop background
  work, and close result channels promptly when the supplied `context.Context`
  is done. A subscriber that ignores cancellation will leak goroutines.
- TLS is terminated somewhere in the deployment (either by this process or by
  a reverse proxy), and cookies used for authentication are `Secure`,
  `HttpOnly`, and use an appropriate `SameSite` value.
- Reverse proxies enforce reasonable URL length, request-body, header, idle,
  read, and write timeouts, and disable response buffering for SSE endpoints.
- Edge infrastructure provides rate limiting and, where needed, per-IP or
  per-user connection limits; `WithMaxConcurrentOperations` only limits the
  total number of concurrent streams for one `Handler` instance, not per
  client.
- The embedder treats errors returned by `Subscriber.Subscribe` as
  potentially client-visible (they are serialized verbatim into the `next`
  event) and sanitizes any backend/internal error text before returning it.
- GET-based subscriptions place `query`, `operationName`, and `variables` in
  the URL. Treat these as non-secret transport metadata: they can be recorded
  in browser history, proxy access logs, and `Referer` headers. Do not put
  credentials, tokens, or other sensitive values in subscription variables.
- Custom `WithCheckOrigin` callbacks implement a strict, scheme-aware,
  explicit origin allowlist rather than a permissive default (e.g. `return
  true`) or a substring/suffix match.

## Known limitations and mitigations

- **Origin checking.** The default origin check (`sameOrigin`) compares only
  the request `Host` against the `Origin` header's host, and additionally
  rejects an `Origin` that downgrades from `https` to `http` on the same host
  when this process terminates TLS directly (`r.TLS != nil`). It does not
  otherwise validate the origin scheme or port, and requests without an
  `Origin` header are allowed (this accommodates same-origin requests from
  older browsers and non-browser clients). Deployments behind a
  TLS-terminating reverse proxy, or that require a stricter policy (explicit
  allowlist, rejecting missing `Origin`, validating `X-Forwarded-Proto`),
  should supply a `WithCheckOrigin` callback. Cookie-authenticated SSE
  requests can otherwise be opened by any origin the deployment allows, so
  also use SameSite cookies and, where possible, prefer bearer authentication
  initiated by `fetch` (native `EventSource` cannot set an `Authorization`
  header, which is why cookie-based auth is common for SSE and why origin
  checks matter).
- **Concurrency and duration limits are global, not per-client.**
  `WithMaxConcurrentOperations` limits the total number of concurrent SSE
  streams served by one `Handler`; it does not prevent a single client or
  source IP from consuming every slot. Configure per-IP/per-user rate
  limiting and connection limits at the reverse proxy or edge, and set
  `WithMaxOperationDuration` to bound how long any single stream can run.
- **Read limits.** `WithReadLimit` bounds both POST request bodies and the
  raw GET query string (rejected with HTTP 413 when exceeded). It does not
  protect against a slow client that sends bytes gradually within the limit;
  configure `http.Server.ReadTimeout`/`ReadHeaderTimeout` (or an equivalent
  reverse-proxy timeout) for defense against slow uploads.
- **Subscriber error messages are returned verbatim to clients.** Unlike the
  panic-recovery path (which logs the recovered value server-side and
  returns a generic `"internal server error"` message), an `error` returned
  from `Subscriber.Subscribe` has its `Error()` text sent directly to the
  client in a `next` event. Only return client-safe messages from your
  `Subscriber` implementation; wrap or log unexpected/internal errors and
  return a generic message instead.
- **SSE framing.** Event names emitted by the handler are fixed constants
  (`next`, `complete`); response bodies are serialized with
  `encoding/json`, which escapes control characters, so user-controlled
  query/variable/result data cannot inject additional SSE fields or events.

SSE streams are long-lived and should be protected with operation duration and
concurrency limits, HTTPS, and normal HTTP rate limiting at the edge.
