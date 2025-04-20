# udplb bpf program

The Go adapter exposes the `BPF` interface implementing basic operations to run,
log and operate an UDPLB BPF program.

## Package internal structure

##### `bpf_{array,map,objects,variable}.go`

These files contain interfaces to conveniently interact with ebpf data structures,
such as `ebpf.Variable` and `ebpf.Map`.

##### `ds_{backends,manager,sessions}.go`

These files contain interfaces to conveniently manage `Backends`, `Sessions` and
`DataStructureManager`.

The `DataStructureManager` implements a single threaded control loop to ensure bpf
kernel data structures are updated in a thread-safe way.

`Backends` and `Sessions` interfaces specifies the Delete, Put and Set methods.
Their concrete implementation interacts with a `DataStructureManager`.

##### `udplb_kern*.c`

The code of the UDPLB BPF program.

##### `udplb_user*.go`

The code that instantiate the BPF program.
