# pkg

Go code shared by agent and control center. Standard library only.

| Package | Purpose |
|---|---|
| `proto` | Agent ↔ control center WebSocket protocol v1: envelope, message types, close codes |
| `sign` | Ed25519 command signing (control center) and verification with time window and replay cache (agent) |
| `jcs` | RFC 8785 canonical JSON for signed messages, restricted to integers |
| `ids` | UUIDv7 generation and validation |

## Signed commands

The signature covers the canonical form of the received JSON object without its
`sig` member. Unknown members are therefore covered too, and re-encoding cannot
change the signed bytes. Signed payloads must not contain fractional numbers;
`jcs` rejects them instead of guessing at ECMAScript number formatting.

## Tests

```bash
go test ./pkg/...
```
