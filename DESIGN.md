# UDPLB Design Document

**Session-aware UDP load balancer: eBPF/XDP fast path, Go slow path, deterministic placement via Reverse Lookup Table.**

## Problem Statement

UDP game servers scale along two axes: UDP packet rate and gRPC connection count.
Multiple load balancer instances front N backend servers and must route packets
by session ID to the correct backend. Standard L4 load balancers cannot inspect
application-layer session IDs. Source-IP hashing fails when one IP carries
multiple sessions or when backends hit capacity between health check intervals.

How does a system route UDP packets by session ID at wire speed, across
independent load balancer nodes, without inter-node coordination for correctness?

UDPLB places an eBPF/XDP program on the NIC that hashes a 128-bit session ID
into a Reverse Lookup Table (RLT). Given the same backend list, all nodes
produce identical lookup tables without consensus. A session map caches
assignments for subsequent packets. A ring buffer notifies Go userspace of new
assignments for optional distributed coordination.

## Tenets

1. **Consistent placement without coordination**: RLT deterministic hashing ensures all nodes route the same session to the same backend without inter-node communication. Each node routes correctly independently -- WAL/consensus optimizes session reuse but correctness does not depend on it. The system tolerates brief misrouting during backend transitions.
2. **Subsequent-packet speed over first-packet speed**: known sessions resolve via a single hash map lookup, skipping ring buffer operations. First packets pay lookup table access and ring buffer notification cost.
3. **Minimal disruption**: backend changes affect only the minimum set of lookup table slots. RLT recomputes with minimal session displacement.
4. **Simplicity over generality**: UDPLB solves one problem well.

## Requirements

- Route UDP packets by 128-bit session ID embedded in a UDPLB datagram header (4-byte prefix + 16-byte UUID).
- Forward packets at wire speed using eBPF/XDP with no kernel network stack traversal.
- Multiple LB instances must route the same session to the same backend without coordination.
- Backend additions, removals, and failures must not disrupt existing sessions beyond a minimal transition window.
- Subsequent packets for known sessions must not incur locking or coordination overhead.
- Packet loss during backend transitions is acceptable at low rates.

## Out of Scope

- IPv6 support (IPv4 only).
- TCP or SCTP load balancing.
- Application-layer health checking (external monitor required).
- TLS termination.
- Session migration between backends.
- Cloud provider integration (no AWS/GCP/Azure-specific code).

## Success Criteria

- Subsequent packets forwarded in < 1 microsecond (XDP fast path, no locks).
- Backend removal displaces < 1/N sessions (where N = number of backends).
- Two independent UDPLB nodes with identical backend lists produce identical lookup tables (zero coordination for correctness).
- Zero packet drops during normal operation (drops acceptable only during backend transitions).
- eBPF program passes kernel verifier on Linux 5.15+.

## Proposed Design

### Architecture

```text
                     +-------------------+
                     |    Client(s)      |
                     +--------+----------+
                              |
                              v
              +-------------------------------+
              |        UDPLB Node(s)          |
              |                               |
              |  +----------+  +-----------+  |
              |  | XDP/eBPF |  | Go        |  |
              |  | fast path|  | slow path |  |
              |  +----+-----+  +-----+-----+  |
              |       |              |         |
              |       +---ring buf---+         |
              |              |                 |
              +-------------------------------+
                     |       |       |
                     v       v       v
              +-------+ +-------+ +-------+
              | BE 0  | | BE 1  | | BE N  |
              +-------+ +-------+ +-------+
```

Three layers compose UDPLB. The kernel layer (XDP/eBPF) handles the fast path:
validate, lookup, rewrite, forward. The userspace layer (Go) handles the slow
path: new session notifications, state changes, coordination. The distributed
layer synchronizes state across UDPLB nodes via WAL consensus. An eBPF ring
buffer connects the kernel and userspace layers.

### Packet Flow

```text
NIC         XDP/eBPF                       Userspace
 |            |                               |
 |--packet--->|                               |
 |            |--must_loadbalance()           |
 |            |  9 checks:                    |
 |            |  ethhdr, ETH_P_IP, iphdr,     |
 |            |  daddr, IPPROTO_UDP, udphdr,  |
 |            |  dport, udpdata, prefix       |
 |            |                               |
 |            |--key = fast_hash(session_id)  |
 |            |        % lookup_table_size    |
 |            |                               |
 |            |--session_map[key] lookup      |
 |            |  [hit] --> backend_idx        |
 |            |  [miss]--> lookup_table[key]->|
 |            |            + ringbuf_submit ->|--new assignment
 |            |            + session_map put  |
 |            |                               |
 |            |--rewrite(h_dest, daddr,       |
 |            |   dport, IP csum, UDP csum)   |
 |<--XDP_TX---|                               |
```

`session_map` and `lookup_table` share the same hash key. The XDP program
computes `fast_hash(session_id) % lookup_table_size` once and uses the result
as key for both maps.

Subsequent packets hit `session_map[key]` and forward directly -- no ring buffer
interaction, no locks. First packets fall through to `lookup_table[key]`, submit
an assignment to the ring buffer, persist the mapping in `session_map`, and
forward. XDP_TX returns the rewritten packet to the same NIC at layer 2, bypassing
the kernel network stack.

### Reverse Lookup Table

The Reverse Lookup Table (RLT) distributes backend slots deterministically.

**Algorithm** (`ReverseCoordinatesLookupTable`): split each backend UUID into
4 x `uint32` coordinates (`NCoordinates = 4`). For each backend and each
coordinate, compute `coordinate % prime` and fill lookup table slots at
multiples of that modular result. Decrease the prime iteratively using Mersenne
prime exponents. Fill remaining slots round-robin.

**Properties**: deterministic (same input produces same output), disruption-minimal
(backend removal affects a bounded set of slots), evenly distributed across
backends, O(m*n) complexity where m = table size and n = backend count.

**Recommended table sizes** (Prime constants):

| Constant     | Value | Max Backends |
|--------------|-------|--------------|
| `Prime307`   | 307   | < 3          |
| `Prime4071`  | 4071  | < 40         |
| `Prime65497` | 65497 | < 650        |

**Alternative algorithms** (defined in `rlt.go`):

1. `ReverseCoordinatesLookupTable` -- coordinate-based, production default.
2. `RobustFibLookupTable` -- Fibonacci-spaced insertion from hash offsets.
3. `RobustSimpleLookupTable` -- sequential insertion from hash offsets.
4. `NaiveFibLookupTable` -- Fibonacci-count round-robin assignment.
5. `SimpleLookupTable` -- modular round-robin (`index % n`).
6. `ShardedLookupTable` -- shard-based (unimplemented).

### Distributed Coordination

UDPLB borrows signal transduction naming from biology.

| Type       | Scope         | Transport        | Consensus |
|------------|---------------|------------------|-----------|
| Autocrine  | Within node   | eBPF ring buffer | N/A       |
| Paracrine  | Between nodes | UDP broadcast    | No        |
| Endocrine  | Between nodes | gRPC/TCP         | Yes (WAL) |

**Assignment flow**:

```text
eBPF           Node A              Network           Node B
  |               |                   |                 |
  |--ring buf---->|                   |                 |
  |               |--UDP broadcast--->|---------------->|  Paracrine
  |               |--WAL propose----->|---------------->|  Endocrine
  |               |                   |                 |
  |               |<--WAL consensus---|<----------------|
  |               |--update eBPF maps |                 |
```

**Write-Ahead Log (WAL)**:

The WAL uses two entry types: `DataWALEntryType` for operations and
`StateWALEntryType` for full state snapshots. Each `WALEntry` contains
`ProposalHash`, `PreviousHash`, and `Hash` (SHA-256) forming a hash chain.

Auto-consent: when entries with the same `Key` and `Data` arrive from different
`Node` IDs within duration D, the WAL auto-consents the first proposal and
discards duplicates. `WALMux` routes entries by `WALName` to the correct WAL
instance.

**DVDS Controller**:

The Distributed Volatile Data Structure controller wraps a `StateMachine` with
a WAL for endocrine signaling. Observers receive the full state on each update,
enabling stateless consumption without maintaining duplicate state.

## Technical Design

### Core Types

| Type           | Kind   | Persisted/Volatile | Description                                      |
|----------------|--------|--------------------|--------------------------------------------------|
| `Backend`      | struct | Volatile           | `Id` (uuid.UUID), `Spec`, `Status`, `coordinates` ([4]uint32) |
| `BackendSpec`  | struct | Persisted          | `IP` (net.IP), `Port` (int), `MacAddr`, `State`  |
| `BackendStatus`| struct | Volatile           | `State` (current backend state)                  |
| `Assignment`   | struct | Volatile           | `BackendId` (uuid.UUID), `SessionId` (uuid.UUID) |
| `Config`       | struct | Persisted          | `Ifname`, `IP`, `Port`, `Backends` ([]BackendConfig) |
| `State`        | int    | Volatile           | Enum: Unknown(0), Available(1), Unschedulable(2), Unavailable(3) |

### State Machine

```text
                  +--[health check]-->
+----------+     |     +-----------+
| Unknown  |-----+     | Available |
+----------+           +-----+-----+
     ^                       |
     |                  [soft limit]
     |                       |
     |                       v
     |               +--------------+
     +--[recovered]--| Unschedulable|
     |               +------+-------+
     |                      |
     |              [hard limit/lost]
     |                      |
     |                      v
     |               +--------------+
     +---------------| Unavailable  |
                     +--------------+
```

- `StateUnknown` (0): backend state is unknown; treat as unavailable.
- `StateAvailable` (1): backend accepts packets for new and existing sessions.
- `StateUnschedulable` (2): backend accepts existing sessions; rejects new ones (soft limit reached).
- `StateUnavailable` (3): backend accepts no packets (hard limit or lost).

**Generic state machines** (package `statemachineadapter`, unexported types):

| Type              | Go Signature                          | Commands               |
|-------------------|---------------------------------------|------------------------|
| `array[T]`        | `StateMachine[T, []T]`                | Append, Put, Delete    |
| `hashmap[E,K,V]`  | `StateMachine[E, map[K]V]`            | Put, Delete            |
| `set[T]`          | `StateMachine[T, map[T]struct{}]`     | Put, Delete            |
| `counter`         | `StateMachine[int64, int64]`          | Add, Subtract          |

All satisfy `StateMachine[T, U]` interface: `Codec` (Encode/Decode) +
`Execute(verb StateMachineCommand, obj T) error` + `State() U` + `DeepCopy()`.

`StateMachineCommand` values: `AddCommand`, `AppendCommand`, `DeleteCommand`,
`PutCommand`, `SubtractCommand`.

### eBPF Maps

| Map                     | BPF Type              | Key              | Value            | Max Entries | Purpose                     |
|-------------------------|-----------------------|------------------|------------------|-------------|-----------------------------|
| `backend_list_a`        | `BPF_MAP_TYPE_ARRAY`  | `BACKEND_INDEX`  | `backend_spec_t` | 64          | Backend specs (buffer A)    |
| `backend_list_b`        | `BPF_MAP_TYPE_ARRAY`  | `BACKEND_INDEX`  | `backend_spec_t` | 64          | Backend specs (buffer B)    |
| `lookup_table_a`        | `BPF_MAP_TYPE_ARRAY`  | `LOOKUP_TABLE_INDEX` | `BACKEND_INDEX` | 1<<16   | Session-to-backend (buffer A) |
| `lookup_table_b`        | `BPF_MAP_TYPE_ARRAY`  | `LOOKUP_TABLE_INDEX` | `BACKEND_INDEX` | 1<<16   | Session-to-backend (buffer B) |
| `session_map_a`         | `BPF_MAP_TYPE_HASH`   | `SESSION_ID_TYPE`| `BACKEND_INDEX`  | 1<<16       | Known sessions (buffer A)   |
| `session_map_b`         | `BPF_MAP_TYPE_HASH`   | `SESSION_ID_TYPE`| `BACKEND_INDEX`  | 1<<16       | Known sessions (buffer B)   |
| `assignment_ringbuf`    | `BPF_MAP_TYPE_RINGBUF`| --               | `assignment_t`   | 36KB        | New assignment notifications|

`BACKEND_INDEX` = `__u32`. `BACKEND_ID_TYPE` = `__u128`. `SESSION_ID_TYPE` = `__u128`.
`LOOKUP_TABLE_INDEX` = `__u32`.

`backend_spec_t` contains: `id` (__u128), `ip` (__u32), `port` (__u16), `mac` ([ETH_ALEN]char).

**Double-buffering**: `active_pointer` (0 or 1) selects the `_a` or `_b` variant.
Note: `backend_list` uses opposite polarity from `lookup_table` and `session_map`
in the C code. Userspace writes to the inactive variant via `SetAndDeferSwitchover`,
then flips the pointer. `DataStructureManager` serializes all writes through an
event loop (1 worker thread) and dispatches watch events through a separate pool
(3 worker threads).

### Package Catalog

```text
cmd/
  udplb/                   Entry point, bootstrap controllers
  udplb_test/              E2E test binary and setup
internal/
  adapter/
    bpf/                   eBPF program, DataStructureManager, double-buffered maps
    bgp/                   BGP route advertisement adapter
    cluster/               Cluster topology (stubs)
    monitor/               Backend monitoring adapter
    rlt/                   Reverse Lookup Table algorithms (6 variants)
    statemachine/          Generic state machines (array, hashmap, set, counter)
    vmagent/               Test VM agent for E2E infrastructure
    wal/                   Write-Ahead Log implementation
  controller/
    dvds/                  Distributed Volatile Data Structure controller
  types/                   Core types, interfaces (Runnable, Watcher, StateMachine, WAL, Cluster)
  util/                    WatcherMux, WorkerPool, RingBuffer, helpers
pkg/
  apis/
    common/v1alpha1/       Common protobuf definitions
    vmagent/v1alpha1/      VM agent protobuf definitions
```

## Design Patterns

**Double-buffering**: backend lists, lookup tables, and session maps use A/B map
pairs. `active_pointer` atomically selects which variant the XDP program reads.
Userspace writes to the inactive variant, then flips the pointer -- zero
read-during-write races on the data path without locks.

**Event-driven dispatch**: the eBPF ring buffer feeds `DataStructureManager`,
which dispatches through `WatcherMux` to registered consumers. An event worker
pool (1 thread) serializes writes. A watch worker pool (3 threads) dispatches
assignment notifications to observers.

**Generic state machines**: all distributed state (arrays, maps, sets, counters)
implements `StateMachine[T, U]` with `Codec` for serialization. The DVDS
controller wraps a state machine with a WAL, providing distributed consensus
over any data structure that satisfies the interface.

## Alternatives Considered

- **Do nothing vs build UDPLB**: without session-aware load balancing, UDP servers cannot scale beyond single-instance packet rate. Clients with the same source IP cannot route to different backends. Rejected.
- **XDP vs DPDK**: chose XDP for kernel integration without dedicated cores. DPDK requires a userspace networking stack and exclusive NIC access.
- **Double-buffered maps vs single maps with locks**: chose double-buffering to avoid read-during-write races on the data path without any locking in the XDP program.
- **Custom WAL vs Raft**: chose custom WAL for domain-specific auto-consent (duplicate keys from different nodes within duration D). Raft adds leader election overhead unsuitable for multi-master topology.
- **Ring buffer vs perf events**: chose ring buffer for lower per-event overhead and shared memory semantics.
- **Lookup table vs consistent hashing**: chose lookup table for exact control over slot distribution and deterministic recomputation. Consistent hashing offers limited control during backend churn.
- **C eBPF vs Rust/aya**: chose C for cilium/ebpf + bpf2go Go ecosystem compatibility. Rust/aya lacks mature Go integration.

## Risks and Mitigations

- **eBPF verifier rejection**: the kernel verifier may reject the XDP program on untested kernel versions. Mitigation: keep kernel code minimal, test on Linux 5.15+ kernels, limit loop bounds.
- **Ring buffer overflow**: a burst of new sessions may overflow the 36KB ring buffer (1024 assignments). Mitigation: overflow drops the notification only -- the packet still forwards via XDP_TX. The session re-assigns on the next packet if not cached.
- **Backend transition window**: sequential map switchover (`backend_list` then `lookup_table` then `session_map`) creates a brief inconsistency window. Mitigation: acceptable by design (tenet 1); the window spans microseconds.
- **WAL/cluster unimplemented**: distributed coordination (WAL, cluster, DVDS) contains stubs and partial implementations. Mitigation: RLT provides correctness without coordination (tenet 1). Single-node and multi-node-without-consensus operation works today.

## Testing Strategy

- **Unit tests** (build tag: `unit`): state machines, `DataStructureManager`, `WatcherMux`. Run via `forge test run unit`.
- **Benchmarks** (build tag: `benchmark`): RLT algorithm variants -- measures consistency across backend additions and removals.
- **Integration tests** (build tag: `integration`): eBPF kernel program loading verification (stub).
- **E2E tests** (build tag: `e2e`): 3 Ubuntu 24.04 VMs via testenv-vm (libvirt). Tests cover SSH reachability, binary deployment, UDP connectivity, and network interface verification. Run via `forge test run e2e`.
- **Lint**: build tag enforcement (`lint-tags`), license header checks (`lint-licenses`), golangci-lint (`lint`).

All tests run via `forge test-all`.

## FAQ

**Why not use consistent hashing instead of a lookup table?**
Consistent hashing provides O(1) lookup but offers limited control over slot
distribution during backend churn. RLT recomputes the full table
deterministically, giving exact control over which sessions move and guaranteeing
identical tables across all nodes.

**Can a single UDPLB node operate without a cluster?**
Yes. RLT produces identical output from identical input, requiring no
coordination. WAL, DVDS, and cluster components optimize session map
synchronization but are not required for correct routing.

**What happens if the ring buffer overflows?**
The packet still forwards (XDP_TX continues). Only the userspace notification
drops. The session re-assigns on the next packet if not already cached in
`session_map`.

**Why double-buffer maps instead of using per-CPU maps?**
Double-buffering allows atomic bulk updates (swap all backends at once) without
partial-state reads. Per-CPU maps solve concurrent access from multiple CPUs --
a different problem than atomic state transitions.

## Appendix

### Build Targets (forge.yaml)

```yaml
build:
  - name: generate-protobuf   # Compile .proto files to Go
  - name: generate-bpf        # Compile eBPF C to Go bindings via bpf2go
  - name: generate-all        # Chain: protobuf + bpf + go generate
  - name: format              # Format Go source
  - name: udplb               # Build binary to ./build/bin/udplb

test:
  - name: lint-tags            # Verify build tag presence
  - name: lint-licenses        # Verify Apache 2.0 license headers
  - name: lint                 # golangci-lint
  - name: unit                 # Unit tests
  - name: e2e                  # End-to-end tests (3 VMs)
```

### UDPLB Datagram Format

4-byte prefix (`0x55554944` = "UUID" in ASCII) + 128-bit session ID + payload.

```text
                  0      7 8     15 16    23 24    31
                 +--------+--------+--------+--------+
                 |  0x55  |  0x55  |  0x49  |  0x44  |
                 |            (0-31 bit)             |
                 +--------+--------+--------+--------+
                 |            Session Id             |
                 |            (32-63 bit)            |
                 +--------+--------+--------+--------+
                 |            Session Id             |
                 |            (64-95 bit)            |
                 +--------+--------+--------+--------+
                 |            Session Id             |
                 |           (96-127 bit)            |
                 +--------+--------+--------+--------+
                 |            Session Id             |
                 |           (128-159 bit)           |
                 +--------+--------+--------+--------+
                 |
                 |          data octets ...
                 +---------------- ...
```

`UDPLB_PACKET_PREFIX` expands to `(0x55 << 24) + (0x55 << 16) + (0x49 << 8) + 0x44`.
The `udpdata` struct holds `prefix` (__u32) and `session_id` (__u128).
