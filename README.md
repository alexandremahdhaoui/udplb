# UDPLB

**Session-aware UDP load balancer using eBPF/XDP for wire-speed packet forwarding.**

> "My UDP servers scale along two axes: packet rate and gRPC connection count.
> Standard L4 load balancers hash by source IP, which breaks when clients share
> an IP or reconnect. I need a load balancer that routes by session ID at wire
> speed and respects backend capacity limits."

## What problem does UDPLB solve?

- UDP servers accept both raw UDP packets and long-lived gRPC connections. A single server reaches capacity on either axis before the other. UDPLB distributes sessions across backends by tracking both.
- Source-IP hashing routes all clients behind the same NAT to one backend. UDPLB hashes a 128-bit session ID embedded in each datagram instead.
- Kernel-bypass networking requires a custom fast path. UDPLB attaches an XDP program that forwards packets before they enter the kernel network stack.
- Multi-instance deployments must agree on session placement without blocking the data plane. UDPLB uses deterministic hashing so each node routes independently, with optional WAL consensus for consistency.

## Table of Contents

- [How do I run UDPLB?](#how-do-i-run-udplb)
- [How does UDPLB route packets?](#how-does-udplb-route-packets)
- [How does UDPLB handle backend failures?](#how-does-udplb-handle-backend-failures)
- [How do multiple UDPLB instances coordinate?](#how-do-multiple-udplb-instances-coordinate)
- [What is the UDPLB datagram format?](#what-is-the-udplb-datagram-format)
- [How do I configure backends?](#how-do-i-configure-backends)
- [How do I build from source?](#how-do-i-build-from-source)
- [Architecture](#architecture)
- [License](#license)

## How do I run UDPLB?

Prerequisites: Go 1.22+, clang, libbpf-dev, linux-headers, [forge](https://github.com/alexandremahdhaoui/forge).

```shell
forge build udplb
```

Create a `config.yaml` (fields match the intended `Config` struct; config file loading is not yet wired into `main.go`):

```yaml
ifname: eth0
ip: 10.0.0.1
port: 9000
backends:
  - enabled: true
    ip: 10.0.0.10
    mac: "aa:bb:cc:dd:ee:01"
    port: 9000
```

```shell
sudo ./build/bin/udplb config.yaml
```

Send a test packet (4-byte prefix `UUID` + 16-byte session ID):

```shell
echo -n "UUID$(head -c 16 /dev/urandom)" | nc -u 10.0.0.1 9000
```

## How does UDPLB route packets?

1. A packet arrives at the NIC. The XDP hook fires before the kernel network stack.
2. `must_loadbalance()` validates 9 conditions: Ethernet bounds, ETH_P_IP, IP bounds, destination IP matches load balancer, IPPROTO_UDP, UDP bounds, destination port matches, datagram bounds, and the 4-byte UDPLB prefix.
3. The XDP program computes `key = fast_hash(session_id) % lookup_table_size`.
4. It looks up `session_map[key]`. On hit, the packet belongs to a known session. On miss, it reads `lookup_table[key]` to assign a backend, notifies userspace via ring buffer, and writes the assignment into `session_map`.
5. The program rewrites Ethernet destination MAC, IP destination address, UDP destination port, and recomputes IP and UDP checksums. It returns `XDP_TX` to send the packet back out the same NIC.

`session_map` and `lookup_table` share the same hash key. The hash runs once per packet. Zero-copy forwarding keeps packets out of the kernel network stack.

## How does UDPLB handle backend failures?

Each backend transitions through four states:

- **Unknown**: initial state; treated as unavailable until health data arrives.
- **Available**: healthy and accepting new sessions.
- **Unschedulable**: healthy but not accepting new sessions (soft limit reached). Existing sessions continue.
- **Unavailable**: down or evicted. No packets forwarded.

When a backend becomes Unschedulable, the load balancer stops assigning new sessions to it. The Reverse Lookup Table recomputes to redistribute new-session slots with minimal disruption. Existing session mappings remain intact until the backend becomes Unavailable.

## How do multiple UDPLB instances coordinate?

UDPLB borrows signal transduction terminology from biology. Three signaling types carry state between components:

| Signal     | Transport | Scope            | Consensus |
|------------|-----------|------------------|-----------|
| Autocrine  | ring buffer | eBPF to userspace (local) | N/A |
| Paracrine  | UDP broadcast | node to node     | No  |
| Endocrine  | gRPC/TCP  | node to node     | Yes (WAL) |

When the XDP program assigns a new session, it writes to the ring buffer (Autocrine). Userspace broadcasts the assignment to peers via UDP (Paracrine). The DVDS controller logs the assignment to the Write-Ahead Log over gRPC (Endocrine) for durable consensus.

Deterministic hashing means each node computes the same lookup table independently. WAL consensus optimizes session reuse but correct routing does not require it.

## What is the UDPLB datagram format?

UDPLB expects a 4-byte prefix (`0x55554944`, ASCII `UUID`) followed by a 128-bit session ID, then the payload.

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

## How do I configure backends?

```yaml
ifname: eth0
ip: 10.0.0.1
port: 9000
backends:
  - enabled: true
    ip: 10.0.0.10
    mac: "aa:bb:cc:dd:ee:01"
    port: 9000
  - enabled: false
    ip: 10.0.0.11
    mac: "aa:bb:cc:dd:ee:02"
    port: 9000
```

`Config` fields:

| Field      | Type     | Description                                |
|------------|----------|--------------------------------------------|
| `ifname`   | string   | Network interface to attach the XDP program |
| `ip`       | string   | IP address the load balancer listens on     |
| `port`     | uint16   | UDP port the load balancer listens on       |
| `backends` | array    | List of `BackendConfig` entries             |

`BackendConfig` fields:

| Field     | Type   | Description                           |
|-----------|--------|---------------------------------------|
| `enabled` | bool   | Whether this backend accepts traffic  |
| `ip`      | string | Backend IP address                    |
| `mac`     | string | Backend MAC address for L2 forwarding |
| `port`    | int    | Backend UDP port                      |

## How do I build from source?

Install system dependencies:

```shell
sudo apt-get install clang libbpf-dev linux-headers-$(uname -r)
```

Requires Go 1.22 or later.

```shell
forge build generate-all
forge build udplb
```

The binary lands in `./build/bin/udplb`.

## Architecture

UDPLB operates across three layers. The kernel layer runs an XDP program for zero-copy packet forwarding at wire speed. The userspace layer runs Go controllers that manage backend state, session tracking, and lookup table recomputation. The distributed layer synchronizes state across nodes using a Write-Ahead Log over gRPC.

See [DESIGN.md](./DESIGN.md) for full architecture, trade-offs, and design decisions.

## License

Apache 2.0. See [LICENSE](./LICENSE).
