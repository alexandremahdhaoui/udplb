# Controller

Controllers are responsible for listening for and reacting to events.

Upon receiving an event, a controller will **react** appropriately by
dropping the event or performing an **operation**.
The whole sequence of internal operation performed by the controller
as described below:

- Apply filters.
- Evaluate the event.
- Perform appropriate response:
  - Perform an operation.
  - Drop event.

Please note that some controllers may or may not implement a control loop.
The simplest controller is the one that starts UDPLB from the command
line.
It is not implemented here but in the main function of `cmd/udplb`.
It will bootstrap other controllers and await for a SIGTERM/SIGINT.
Upon receiving such events it will gracefully shutdown other controllers.

## Sequence

Below will be listed a sequence of events and operations commonly found
in UDPLB.

- [Ev] Start UDPLB.
- [Op] Init new bpfadapter.
- [Op] Initialize bpf data structures.
- [Op] Run bpf program.
- [Op] Start monitoring backends.
- [Op] Start monitoring local assignments.

- [Ev] Monitoring event shows up.
- [Op] Agree on a new state b/w loadbalancers.
- [Ev] Create an internal consensus event.

- [Ev] Consensus: Backend XYZ becomes unavailable or unschedulable.
- [Op] Update bpf data structures (e.g. adding/removing a backend).

- [Ev] New assignment made locally.
- [Op] Notify other loadbalancers.

- [Ev] Another loadbalancer notifies a new assignment.
- [Op] Fast sync assignments locally.

- [Ev] SIGINT/SIGTERM
- [Op] Gracefully shutdown application by propagating `Close()`.

## Data types

There are 2 types of data controllers manages:

- Persisted data:
  - Any data that can be obtained from diverse persistent source of data.
  - E.g.: a kubernetes services and the udplb configuration are of type `persisted`.
  - Does not require any consensus, the source of truth reside outside UDPLB or
    watching `ListAndWatch()`-ing the source of truth.
  - Data consistency can be achieved by simply restarting UDPLB.
- Volatile or runtime data:
  - Any data that is acquired or defined by UDPLB itself.
  - E.g.: both `Assignment` and `BackendState` data kind are of type `volatile`.
  - Data consistency must be achieved via consensus.

Questions:

- How is a data kind of type `volatile` synchronized to a new or restarted UDPLB node?

## Data kinds

A "data kind" is a kind of data, such as `BackendSpecList`, `Assignment` or `BackendState`.
The former is a data kind of type `persisted`.
The 2 latters are data kinds of type `volatile`.

## Signal transduction

All data types (persisted or volatile) may be subject to signal transduction of
varied types.

Additionally, one data kind may use more than one signalling pathway. These different pathways may
use different signal transduction types.

There are 3 different types of signal transduction:

- **Autocrine**:
  - fastest.
  - B/w userland and bpf program of one UDPLB node.
  - We could also consider "autocrine" all internal communication b/w controllers.
- **Paracrine**
  - fast.
  - B/w UDPLB nodes.
  - Via UDP.
  - No consensus.
- **Endocrine**
  - Slower.
  - B/w UDPLB nodes.
  - Require consensus.
  - over TCP.
  - via GRPC.

### Example: Transduction of the `Assignment` data kind

The `Assignment` data kind is of data type `volatile` and possesses 3 different
signalling pathways:

- **LocalAssignment**
  - Signal transduction type: `Autocrine`.
  - Goal: Signal userland about a new assignment discovered in bpf program.
  - It triggers the `RemoteAssignment` and `AssignmentLog` pathways.
- **RemoteAssignment**:
  - Signal transduction type: `Paracrine`.
  - Goal: quickly signal other nodes about a new assignment.
- **AssignmentMap**
  - Signal transduction type: `Endocrine`.
  - Goals:
    - Agree on a consistent log of `Assignment`.
    - Periodically sync `Assignment` data kind b/w Nodes to ensure consistency and purge the log.
    - Sync `Assignment` to new or restarted UDPLB nodes.

### Signal pathways per data kind

Each data kind flows through one or more signal pathways. The table below
lists the signal type, pathway, producer, and consumer for each data kind.

| Data Kind | Signal Type | Pathway | Producer | Consumer |
|---|---|---|---|---|
| Assignment | Autocrine | LocalAssignment (eBPF ring buffer) | `bpf.DatastructureManager` | DVDS controller |
| Assignment | Paracrine | RemoteAssignment (UDP) | `monitoradapter.RemoteAssignment` | DVDS controller |
| Assignment | Endocrine | AssignmentMap (WAL + cluster consensus) | DVDS controller | DVDS controller |
| BackendSpec | Config | BackendSpecList (file read at startup) | `monitoradapter.backendSpecList` | `bpf.DatastructureManager` |
| BackendStatus | Paracrine | BackendStateMap (UDP health probe) | `monitoradapter.backendState` | DVDS controller |
| RLT | Endocrine | ReverseLookupTable (WAL + cluster, gRPC/TCP) | DVDS controller | DVDS controller |

**Assignment** uses 3 pathways. LocalAssignment detects new sessions inside
the eBPF program and delivers them to userspace via a ring buffer.
RemoteAssignment broadcasts assignments to peer nodes over UDP without
consensus. AssignmentMap achieves cluster-wide consistency through WAL
consensus.

**BackendSpec** is read once from the configuration file at startup. No
consensus is required.

**BackendStatus** is produced by periodic UDP health probes against each
backend. The DVDS controller consumes these status events to update the
backend state machine.

**RLT** (Reverse Lookup Table) propagates through WAL-based consensus over
gRPC/TCP. Nodes agree on a consistent RLT state before applying changes.

### Adapter-to-interface mapping

Each adapter implements one or more `types` interfaces and handles a
specific signal type. The table below maps each adapter to its interface
and transport.

| Adapter | Interface | Signal Type |
|---|---|---|
| `bpf.DatastructureManager` | `Watcher[Assignment]` | Autocrine (eBPF ring buffer) |
| `monitoradapter.RemoteAssignment` | `Watcher[Assignment]` + `Runnable` | Paracrine (UDP) |
| `monitoradapter.backendSpecList` | `Watcher[BackendSpecMap]` | Config (file read) |
| `monitoradapter.backendState` | `Watcher[BackendStatusEntry]` + `Runnable` | Paracrine (UDP probe) |
| `waladapter.wal[T]` | `WAL[T]` (`Runnable` + `Watcher[[]T]`) | Endocrine (cluster consensus) |
| `clusteradpater.clusterMux` | `RawCluster` | Network transport (UDP) |
| `clusteradpater.typedCluster[T]` | `Cluster[T]` | Generic typed transport |

Adapters that implement `Runnable` start background goroutines via
`Run(ctx)`. Adapters that implement only `Watcher[T]` provide a `Watch()`
method without requiring `Run`.

### Component lifecycle

All `Runnable` components follow the `terminateCh`/`doneCh` pattern. This
pattern originates from `bpf/manager.go` and applies to every adapter and
controller with a background event loop.

**Channels:**

- `terminateCh` (`chan struct{}`): Signals the component to stop. Created
  in the constructor. Closed by `Close()`.
- `doneCh` (`chan struct{}`): Confirms the component has stopped. Created
  in the constructor. Closed by the event loop after cleanup.

**`Run(ctx)`:**

1. Lock mutex.
2. Guard: return `ErrAlreadyRunning` if running. Return
   `ErrCannotRunClosedRunnable` if closed.
3. Start event loop goroutine(s).
4. Set `running = true`.
5. Unlock mutex and return nil.

**`Close()`:**

1. Lock mutex (defer unlock).
2. Guard: return `ErrRunnableMustBeRunningToBeClosed` if not running.
   Return `ErrAlreadyClosed` if closed.
3. `close(terminateCh)` -- triggers event loop shutdown.
4. Await `doneCh` with a 5-second timeout.
5. Set `running = false`, `closed = true`.

**Event loop goroutine:**

1. Select on `terminateCh` and data channels.
2. On `terminateCh` closure: release resources (close listeners, cancel
   watchers).
3. Close `doneCh` as the last action before returning.

```text
   caller          component          goroutine(s)
     |                 |                   |
     |--- Run(ctx) --->|                   |
     |                 |--- go eventLoop ->|
     |<--- nil --------|                   |
     |                 |                   |--- select { terminateCh, data }
     |                 |                   |
     |--- Close() ---->|                   |
     |                 |-- close(terminateCh)
     |                 |                   |--- cleanup resources
     |                 |                   |--- close(doneCh)
     |                 |<-- doneCh closed -|
     |<--- nil --------|                   |
```

This pattern prevents double-run, run-after-close, double-close, and
close-before-run. The 5-second timeout in `Close()` prevents indefinite
hangs if the event loop fails to terminate.
