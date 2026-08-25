# Cross-stack interop timeline

Language-neutral event names for live interop and peer debugging.

Any stack (Go, Python RNS, MeshChatX, NomadNet probes, WASM ports) can emit the same timeline. Consumers include the Go harness under tests/interop/harness/ and future dump or log tools.

## Wire shape

One JSON object per line.

On stderr (preferred when a parent process captures the child):

```text
INTEROP_EVENT {"ts":"RFC3339","src":"py","event":"ready","kind":"","detail":""}
```

Optional file append when INTEROP_EVENTS_PATH is set. When a Go harness owns the file it sets INTEROP_EVENTS_GO_OWNED=1 so the child only emits on stderr and the parent writes events.jsonl.

Field rules:

- No em dashes in string values
- No semicolons in string values
- src is go or py (or another short stack id)
- event uses the names below
- kind is optional except on fail
- detail is a short human string
- Extra keys may live under fields (object)

## Event names

| Event | Meaning |
|-------|---------|
| ready | Peer finished local init and is waiting for mesh work |
| path_wait | Starting path discovery or identity recall |
| path_req | Path request sent |
| path_resp | Path response or announce answer observed |
| path_ok | Path to target is known |
| node | Selected destination hash (often NomadNet) |
| link_up | Link established |
| link_ok | Link plus application success (page fetch, echo) |
| request_ok | Request or resource completed as expected |
| spawn | Parent started a child probe |
| fail | Terminal failure |

## Fail kinds

| Kind | Meaning |
|------|---------|
| spawn | Could not start peer |
| ready | Never reached ready |
| announce | No announce or announce wait failed |
| path | Path discovery failed |
| identity | Identity recall failed |
| link | Link failed |
| request | Request or page or resource failed |
| timeout | Deadline without a more specific kind |
| harness | Test or harness error |

## Go constants

See pkg/timeline for the same names as Go constants.

## Python helper

tests/interop/py/interop_events.py implements emit(event, **fields).
