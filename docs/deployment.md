# How do I deploy UDPLB?

Step-by-step guide to deploy UDPLB with BGP ECMP across multiple load balancers.

## Network topology

```text
                        10.0.0.0/24 (flat L2)
  +-----------------------------------------------------------+
  |                                                           |
  |  +--------+        +------+  +------+  +------+          |
  |  | Router |  BGP   | LB-0 |  | LB-1 |  | LB-2 |         |
  |  | .0.2   |<------>| .0.10|  | .0.11|  | .0.12|         |
  |  | AS 6500|  ECMP  |AS 6501  |AS 6501  |AS 6501         |
  |  +--------+        +------+  +------+  +------+          |
  |      ^              VIP .0.200 on lo (all 3)              |
  |      |                  |         |         |             |
  |  +--------+        +------+  +------+  +------+          |
  |  | Client |        | BE-0 |  | BE-1 |  | BE-2 |          |
  |  | .0.100 |        | .0.20|  | .0.21|  | .0.22|          |
  |  +--------+        | :8080|  | :8080|  | :8080|          |
  |                     +------+  +------+  +------+          |
  +-----------------------------------------------------------+
```

All nodes share one L2 segment. The router runs BGP (AS 65000) and peers with 3 load balancers (AS 65001). Each LB announces the VIP (`10.0.0.200/32`) via BGP. The router installs a 3-way ECMP route and distributes client traffic across all LBs. Each LB runs an XDP program that forwards packets to backends at L2.

Replace IPs, AS numbers, interface names, and backend count to match your environment.

## Prerequisites

**Kernel**: Linux 5.15+ (XDP and eBPF ring buffer support).

**Packages on all nodes:**

```shell
sudo apt-get install iproute2
```

**Packages on load balancers and router:**

```shell
sudo apt-get install frr
```

**Packages on the build machine:**

```shell
sudo apt-get install clang libbpf-dev linux-headers-$(uname -r)
```

Go 1.22+ and [forge](https://github.com/alexandremahdhaoui/forge) are required to build.

**Capabilities**: `udplb` requires `CAP_BPF`, `CAP_NET_ADMIN`, and `CAP_SYS_ADMIN` (or run as root).

## Step 1: Build

```shell
forge build generate-all
forge build udplb
```

The binary lands in `./build/bin/udplb`. Copy it to each load balancer.

## Step 2: Configure the network

### Router

Enable IP forwarding, disable reverse-path filtering, and disable ICMP redirects:

```shell
sysctl -w net.ipv4.ip_forward=1
sysctl -w net.ipv4.conf.all.rp_filter=0
sysctl -w net.ipv4.conf.default.rp_filter=0

# send_redirects uses OR(all, interface) -- disable on every interface
for f in /proc/sys/net/ipv4/conf/*/send_redirects; do echo 0 > "$f"; done
```

Clear firewall rules that may block forwarding:

```shell
nft flush ruleset || true
iptables -P FORWARD ACCEPT || true
iptables -F FORWARD || true
ufw disable || true
```

### Load balancers

Assign the VIP to the loopback interface and configure ARP to prevent the LB from responding to ARP requests for the VIP on the physical interface:

```shell
ip addr add 10.0.0.200/32 dev lo

sysctl -w net.ipv4.conf.all.arp_ignore=1
sysctl -w net.ipv4.conf.all.arp_announce=2
```

`arp_ignore=1`: reply only if the target IP is configured on the incoming interface. This prevents the LB from answering ARP for the VIP on `eth0` (the VIP lives on `lo`).

`arp_announce=2`: use the best local address as the ARP source. This prevents ARP replies from leaking the VIP as a source address on the physical interface.

### Backends

Ensure firewalls allow inbound UDP on the backend port:

```shell
nft flush ruleset || true
iptables -P INPUT ACCEPT || true
iptables -F INPUT || true
ufw disable || true
```

### Clients

Route VIP traffic through the router:

```shell
ip route add 10.0.0.200/32 via 10.0.0.2

# Disable ICMP redirect acceptance so the kernel doesn't shortcut the router
for f in /proc/sys/net/ipv4/conf/*/accept_redirects; do echo 0 > "$f"; done
```

## Step 3: Configure BGP (FRR)

### Router

Enable BGP and zebra daemons:

```shell
sed -i 's/^bgpd=no/bgpd=yes/' /etc/frr/daemons
sed -i 's/^zebra=no/zebra=yes/' /etc/frr/daemons
```

Write the FRR configuration. The router peers with all LBs and enables ECMP with `maximum-paths`:

```shell
cat > /etc/frr/frr.conf << 'EOF'
frr defaults traditional
hostname router
router bgp 65000
 no bgp ebgp-requires-policy
 bgp router-id 10.0.0.2
 neighbor 10.0.0.10 remote-as 65001
 neighbor 10.0.0.11 remote-as 65001
 neighbor 10.0.0.12 remote-as 65001
 address-family ipv4 unicast
  maximum-paths 3
 exit-address-family
EOF

chown frr:frr /etc/frr/frr.conf
chmod 0640 /etc/frr/frr.conf
systemctl restart frr
```

### Load balancers

Each LB runs FRR in AS 65001, peers with the router, and announces the VIP:

```shell
sed -i 's/^bgpd=no/bgpd=yes/' /etc/frr/daemons
sed -i 's/^zebra=no/zebra=yes/' /etc/frr/daemons
```

Write the FRR configuration. Replace `<LB_HOSTNAME>` and `<LB_IP>` per node:

```shell
cat > /etc/frr/frr.conf << 'EOF'
frr defaults traditional
hostname <LB_HOSTNAME>
router bgp 65001
 no bgp ebgp-requires-policy
 bgp router-id <LB_IP>
 neighbor 10.0.0.2 remote-as 65000
 address-family ipv4 unicast
  network 10.0.0.200/32
 exit-address-family
EOF

chown frr:frr /etc/frr/frr.conf
chmod 0640 /etc/frr/frr.conf
systemctl restart frr
```

Example values per node:

| Node | hostname | router-id |
|------|----------|-----------|
| LB-0 | lb-0    | 10.0.0.10 |
| LB-1 | lb-1    | 10.0.0.11 |
| LB-2 | lb-2    | 10.0.0.12 |

## Step 4: Verify BGP convergence

On the router, confirm all 3 peers show `Established`:

```shell
vtysh -c 'show bgp summary'
```

Check the kernel routing table for 3-way ECMP:

```shell
ip route show 10.0.0.200
```

Expected output (3 nexthops):

```text
10.0.0.200 proto bgp metric 20
        nexthop via 10.0.0.10 dev eth0 weight 1
        nexthop via 10.0.0.11 dev eth0 weight 1
        nexthop via 10.0.0.12 dev eth0 weight 1
```

## Step 5: Configure and start UDPLB

Create a config file on each load balancer. The `mac` field is the backend's L2 address -- UDPLB rewrites `eth.h_dest` to forward at L2 via XDP_TX.

Get each backend's MAC:

```shell
# Run on each backend (or query remotely via SSH)
ip link show eth0 | grep 'link/ether' | awk '{print $2}'
```

Write the config:

```yaml
# /etc/udplb/config.yaml
ifname: eth0
ip: 10.0.0.200
port: 12345
backends:
  - enabled: true
    ip: 10.0.0.20
    mac: "52:54:00:aa:bb:01"
    port: 8080
  - enabled: true
    ip: 10.0.0.21
    mac: "52:54:00:aa:bb:02"
    port: 8080
  - enabled: true
    ip: 10.0.0.22
    mac: "52:54:00:aa:bb:03"
    port: 8080
```

| Field    | Description |
|----------|-------------|
| `ifname` | Network interface where XDP attaches. Must be the interface facing clients and backends. |
| `ip`     | The VIP. Packets with this destination IP are load-balanced. |
| `port`   | UDP port. Packets with this destination port are load-balanced. |
| `backends[].mac` | Backend MAC address. Required for L2 forwarding via XDP_TX. |

Start UDPLB:

```shell
sudo ./udplb /etc/udplb/config.yaml
```

UDPLB attaches an XDP program to the specified interface and begins forwarding matching UDP packets to backends.

## Step 6: Verify forwarding

Send a test packet from a client:

```shell
echo -n "UUID$(head -c 16 /dev/urandom)" | nc -u 10.0.0.200 12345
```

Check XDP attachment on a load balancer:

```shell
ip link show eth0 | grep xdp
```

Read BPF trace logs:

```shell
sudo cat /sys/kernel/debug/tracing/trace_pipe
```

## How does the packet flow?

```text
Client                Router               LB (XDP)              Backend
  |                     |                     |                     |
  |-- UDP to VIP:12345->|                     |                     |
  |                     |-- ECMP fwd -------->|                     |
  |                     |   (dst MAC = LB)    |                     |
  |                     |                     |-- XDP validates:    |
  |                     |                     |   daddr == VIP?     |
  |                     |                     |   proto == UDP?     |
  |                     |                     |   port == 12345?    |
  |                     |                     |   prefix == UDPLB?  |
  |                     |                     |                     |
  |                     |                     |-- session lookup    |
  |                     |                     |   hash(UUID) % size |
  |                     |                     |                     |
  |                     |                     |-- rewrite:          |
  |                     |                     |   h_dest = BE MAC   |
  |                     |                     |   daddr  = BE IP    |
  |                     |                     |   dport  = BE port  |
  |                     |                     |                     |
  |                     |                     |-- XDP_TX ---------->|
  |                     |                     |   (L2 on same net)  |
```

1. Client sends a UDP packet to VIP `10.0.0.200:12345`.
2. Router matches the BGP route and ECMP-forwards to one LB based on 5-tuple hash.
3. The XDP program on the LB validates the packet (9 checks), looks up the session, and rewrites Ethernet destination MAC, IP destination, and UDP port to point at the selected backend.
4. `XDP_TX` sends the rewritten packet back out the same NIC. The L2 switch delivers it to the backend by destination MAC.

## Persist sysctl settings

To survive reboots, add sysctl settings to `/etc/sysctl.d/`:

**Router** (`/etc/sysctl.d/99-udplb-router.conf`):

```ini
net.ipv4.ip_forward = 1
net.ipv4.conf.all.rp_filter = 0
net.ipv4.conf.default.rp_filter = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv4.conf.default.send_redirects = 0
```

**Load balancers** (`/etc/sysctl.d/99-udplb-lb.conf`):

```ini
net.ipv4.conf.all.arp_ignore = 1
net.ipv4.conf.all.arp_announce = 2
```

**Clients** (`/etc/sysctl.d/99-udplb-client.conf`):

```ini
net.ipv4.conf.all.accept_redirects = 0
net.ipv4.conf.default.accept_redirects = 0
```

## Why are these settings needed?

| Setting | Node | Purpose |
|---------|------|---------|
| `ip_forward=1` | Router | Forward packets between interfaces |
| `rp_filter=0` | Router | Allow asymmetric return paths (backend replies bypass the LB) |
| `send_redirects=0` | Router | Prevent ICMP redirects that shortcut ECMP routing |
| `arp_ignore=1` | LB | Prevent ARP replies for VIP on the physical interface |
| `arp_announce=2` | LB | Prevent ARP source address leaking the VIP |
| `accept_redirects=0` | Client | Ignore ICMP redirects to preserve routing through the router |
