# udplb

UDPLB is an opiniated UDP Loadbalancer leveraging eBPF & XDP.

## Goals

The goal of `udplb` is to loadbalance traffic from a customer to the right
UDP server. Following is an excerpt of (another_repo/)cmd/basic/eventreq-srv/main.go
describing why we need this loadbalancer.

        NB: we'll need an advanced loadbalancer (and scheduler) that accounts for
        the number of opened "connexions" and the average load of each backends.

        We may want to hash source ip to let customer stick to the same server and
        avoid creating many redundant cnx. <- Trivial implementation will not work.
        If a packet comes to LB before the server notifies LB it reached limits
        then we might perpetually send a customer to a full server.
        The above scenario can be prevented by using both a "soft" and a hard limit,
        and notifying the scheduler once the soft limit is reached.
        That setup might cause underutilization of resources, and does not prevent
        cases where more than `(hardLimit - softLimit)` new connexions are opened.
        (There is no such things as opening an UDP connexion, but the wording should
        well illustrate the loadbalancing technique).

        Might have to write my own eBPF/XDP loadbalancer to open UDP packets, check
        for a sessionId and forward packet to the right backend.
        The userspace LB process will also need to coordinate with backends to
        loadbalance according to their current load & capacity.

## Backend UDP Server capacity

An UDP server has 2 scalability axis:

- Rate of UDP packet/second.
- Number of opened backend GRPC cnx.

These 2 metrics will help us calculate if the UDP server has capacity for more
incoming requests.

Once an UDP server's capacity decrease below a threshold, the loadbalancer will
not send new traffic ("first-packets") to it.

Hence UDPLB should loadbalance request to UDPs accordingly.
Q: how will UDPLB be notified of backends capacity?
Q: How is backend capacity data synchronized b/w the backend & the load balancer.
Q: How is backend capactity data synced b/w each loadbalancers.

## Specification

**`MUST`**

- In any case a packet MUST not be forwarded to the wrong backend.
  (this would end up creating a new GRPC cnx to the gameproc server.)
- "Subsequent" packets MUST NOT be subject to locking
- "Subsequent" packets MUST be forwarded as soon as possible.

**Plasticity**

- A "first packet" can take time to be served.
- We can await for locks and that data structures are properly
 synced b/w LBs or b/w Backend/LB.
- Sending packets to an unavailable backend is acceptable. In other words,
packet loss is accepted if the rate is low and only occurs shortly after
a backend became unavailable.

To be considered:

- A backend can be removed or lost at any time.
- A backend can be added at any time.
- A loadbalancer can be removed or lost at any time.
- A loadbalancer can be added at any time.
- 2 "first-packets" may hit 2 different loadbalancers at the same time.
  (thus 2 loadbalancers should forward them to the same backend)

TODO: what about loadbalancing same `sessionId` packets to 2 different backends after one of
the 2 backends has reached max capacity?

### Packet specification

**IPV4 header as specified in rfc791**
-> does not change

```
    0                   1                   2                   3
    0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
   |Version|  IHL  |Type of Service|          Total Length         |
   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
   |         Identification        |Flags|      Fragment Offset    |
   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
   |  Time to Live |    Protocol   |         Header Checksum       |
   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
   |                       Source Address                          |
   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
   |                    Destination Address                        |
   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
   |                    Options                    |    Padding    |
   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
   |          data octets ...
   +---------------- ...
```

**UDP header format as specified in rfc768**
-> does not change

```
                  0      7 8     15 16    23 24    31
                 +--------+--------+--------+--------+
                 |     Source      |   Destination   |
                 |      Port       |      Port       |
                 +--------+--------+--------+--------+
                 |                 |                 |
                 |     Length      |    Checksum     |
                 +--------+--------+--------+--------+
                 |
                 |          data octets ...
                 +---------------- ...
```

**UDPLB's datagram header format**

UDPLB expects a 4 byte prefix and a session Id of length 16 bytes (128 bits).

```
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

### Recap

"First-packets" can be slow and the solution can afford locking and syncing
data structures distributed among the loadbalancers.

Subsequent packets must be forwarded ASAP, not be subject to locks, and must not
be forwarded to a wrong backend. They can however be sent to a backend that was
recently down.

## Solution

Let `availableBackends` the set of available backends.

Let `backendsWithCapacity` the number of backends with capacity.

Let `backendsWithCapacityMap` a data structure mapping integers to a backend. E.g. the key
can be the modulo of the sessionId against a multiple of the number of backends.

Let `knownSessionBackendMap` the mapping of known sessionIds to their backend.
This map is useful in order to consistently map a session to a backend even after the backend
was removed from `backendsWithCapacity`.

### Scenario 1. First-packets

#### One LB receive a 1st-packet

- The modulo of the sessionId is computed against a multiple of `backendsWithCapacity`.
- A backend is chosen by accessing the `backendsWithCapacityMap` with the modulo.
- The key-value pair `sessionId`-`backend` is inserted into the hashmap
`knownSessionBackendMap`.
- Sync `knownSessionBackendMap` w/ all loadbalancers.

The backend choice based on the hashing+modulo should be fairly consistent unless
backendsWithCapacity is not consistent.
Hence we must ensure that backendsWithCapacity are.

#### Many LB receives 1st-packets w/ same sessionId simultaneously

### Scenario 2. Subsequent packets

### Scenario 3. Backend becomes down

#### Backend becomes down definitevely (same as backend is removed)

#### Backend becomes down & recovers

### Scenario 4. Backend is added

#### 1 or Many new backends is added

#### Backends are replaced (rollout or blue-green)

### Scenario 5. Garbage collect "closed" SessionIds

## Resources

- <https://ebpf-go.dev/guides/getting-started/>
- <https://storage.googleapis.com/gweb-research2023-media/pubtools/2904.pdf>
- <https://github.com/davidcoles/xvs>
- <https://eunomia.dev/en/tutorials/42-xdp-loadbalancer/>

## Prerequisites

```shell
# install clang + llvm
sudo apt-get install clang-format clang-tidy clang-tools clang clangd libc++-dev libc++1 libc++abi-dev libc++abi1 libclang-dev libclang1 liblldb-dev libllvm-ocaml-dev libomp-dev libomp5 lld lldb llvm-dev llvm-runtime llvm python3-clang

# install libbpf
sudo apt install libbpf-dev

# install kernel headers
sudo apt install linux-headers-`uname -r`
sudo ln -s /usr/include/x86_64-linux-gnu/asm /usr/include/asm

# bpf2go (actually optional)
go get github.com/cilium/ebpf/cmd/bpf2go

# protoc
sudo apt install -y protobuf-compiler
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

## Run ebpf program

```shell
# -- go run
CGO_ENABLED=0 IFNAME=lo go run -exec 'sudo -E' ./cmd/basic/udplb

# -- Make
make -C cmd/basic/udplb run
```

To read bpf trace logs

```shell
sudo cat /sys/kernel/debug/tracing/trace_pipe
```

## Test the EBPF program

```
# -- UNIT TEST
echo "unimplemented"

# -- E2E TEST
make -C cmd/basic/udplb e2e
```
