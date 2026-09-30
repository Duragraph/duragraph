# v2 MCP endpoint

`POST /mcp` (also `/mcp/`) serves stateless Streamable HTTP JSON-RPC. Send
`Content-Type: application/json`, `Accept: application/json, text/event-stream`,
and `Authorization: Bearer <platform-session-JWT>` on **every** POST. The
server returns JSON for requests, 202 with an empty body for notifications
and client responses. It does not issue `Mcp-Session-Id`; GET returns 405 and
DELETE returns 404. The supported protocol versions are `2025-11-25`,
`2025-06-18`, and `2024-11-05`. Clients may send `MCP-Protocol-Version` on
subsequent requests.

The route fails closed unless the platform database and JWT secret are
configured. The live user **and** tenant must be approved, and the tenant's
`db_name` must match the v2 server's configured tenant database. A platform
session cookie is not accepted: MCP tool calls require an explicit Bearer
credential. Browser clients sending `Origin` also require a matching
`DURAGRAPH_BASE_URL`; never use an untrusted Host header as an Origin allowlist.

Capabilities: `ping`, `resources/list`, `resources/read` (assistants and server
info), `tools/list`, and `tools/call` for `invoke_assistant_<uuid>`. Invocation
queues a v2 run through the existing transactional-outbox run endpoint and
returns its run ID (not a completed run). `input` and optional `thread_id` are
supported; `config` is rejected rather than silently ignored because v2 run
creation does not persist it. Unlike legacy, arbitrary legacy registry tools
are **not** advertised: v2 has no corresponding tool registry.
