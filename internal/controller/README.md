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

- [Ev] Monitoring event shows
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

A "data kind" is a kind of data, such as `LiveConfig`, `Assignment` or `BackendState`.
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
- **AssignmentLog**
  - Signal transduction type: `Endocrine`.
  - Goals:
    - Agree on a consistent log of `Assignment`.
    - Periodically sync `Assignment` data kind b/w Nodes to ensure consistency and purge the log.
    - Sync `Assignment` to new or restarted UDPLB nodes.
