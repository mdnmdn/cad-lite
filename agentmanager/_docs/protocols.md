# Protocols

One section per wire protocol or protocol-like component.

## MCP (package `mcp`)

In-process MCP server for injecting tools (the `cad` drawing tools) into agents. stdlib `net/http`,
streamable HTTP without SSE or sessions. One server per app, one token per server, shared by every
agent session.

- **Endpoint.** `Start()` binds `127.0.0.1:0` (tcp4) before returning and yields `Endpoint{URL, Token}`,
  `URL = http://127.0.0.1:PORT/mcp`, `Token` = 32 random bytes hex. Callers inject it as
  `agentmanager.MCPServer{Name, URL, Token}` (header `Authorization: Bearer <token>`). Tokens are never logged.
- **Checks, in order.** `Host` must be `127.0.0.1:PORT` or `localhost:PORT`, and any `Origin` must have a
  loopback host (else `403`); then bearer token via `subtle.ConstantTimeCompare` (else `401`, no OAuth
  metadata). Only `POST /mcp` is served: `GET`, `DELETE` and others get `405` (no server-initiated stream,
  no `Mcp-Session-Id`). Body limit 4 MiB (`413`).
- **Framing.** JSON-RPC 2.0 in the POST body, `application/json` reply. A single request gets one reply; a
  batch (JSON array) gets an array of the replies to its requests; notifications and client responses get no
  reply, and a body with nothing to reply to gets a bare `202`. Bad JSON is `-32700` (id `null`), missing method
  `-32600`, unknown method `-32601`, bad `tools/call` params `-32602`.
- **Methods.** `initialize` echoes the client's `protocolVersion` if it is `2025-06-18`, `2025-03-26` or
  `2024-11-05`, else `2025-06-18`; `capabilities:{tools:{listChanged:false}}`, `serverInfo{name,version}`
  and optional `instructions`. `notifications/*` -> 202. `ping` -> `{}`. `tools/list` (registration order,
  default schema `{"type":"object"}`). `tools/call {name, arguments}`.
- **Tool results.** `{content:[{type:"text",text}|{type:"image",data,mimeType}], isError, structuredContent?}`.
  Unknown tool, handler error, and handler panic are in-band `isError` results with the message as text,
  not JSON-RPC errors.
- **Concurrency.** Handlers run on the request goroutine with `r.Context()` (cancelled on client disconnect
  and on `Close`). `Options.Serialize` (default true) serialises handler calls with a mutex.
- **Lifecycle.** `Close()` stops accepting, waits up to 2 s for in-flight requests, then cancels their
  contexts and force-closes. Idempotent; a closed server cannot be restarted.
