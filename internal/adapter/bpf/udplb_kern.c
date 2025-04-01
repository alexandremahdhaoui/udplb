/*
 * Copyright 2025 Alexandre Mahdhaoui
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// clang-format off
//go:build ignore
#include <linux/bpf.h>
// clang-format on

#include <bpf/bpf_endian.h>
#include <bpf/bpf_helpers.h>
#include <linux/byteorder/little_endian.h>
#include <linux/if_ether.h>

#include "udplb_kern_helpers.c"

/**************************************************************************
 * FOR LAYER 2 LOADBALANCING:
 * https://docs.ebpf.io/linux/program-type/BPF_PROG_TYPE_XDP/
 *
 *   The packet can be redirected to egress on a different interface than
 *   where it entered (like XDP_TX but for a different interface).
 *   This can be done using the bpf_redirect helper (not recommended) or
 *   the bpf_redirect_map helper in combination with a BPF_MAP_TYPE_DEVMAP
 *   or BPF_MAP_TYPE_DEVMAP_HASH map.
 **************************************************************************/

// The loadbalancer's IP is checked to decide if a packet must be loadbalanced.
// UDPLB_IP must be specified by using host's endianness.
volatile const __u32 UDPLB_IP;

// The loadbalancer's port is checked to decide if a packet must be
// loadbalanced.
volatile const __u16 UDPLB_PORT;

// Number of backends defined in the "backends" bpf map.
//
// To add, remove or disable a backend in the "backends" bpf map the
// "n_backends" variable along the map must be locked.
volatile __u32 n_backends;

struct backend_spec {
    __u32 ip;
    __u16 port;
    unsigned char mac[ETH_ALEN];
    _Bool enabled;
};

// The set of all the loadbalancer backends.
//
// To add, remove or disable a backend in the "backends" bpf map the
// "n_backends" variable along the map must be locked.
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 64); // Max 64 backends.
    __type(key, __u32);
    __type(value, struct backend_spec);
} backends SEC(".maps");

// computes the hash of x of size y and return its z modulo
#define hash_modulo(x, y, z) fast_hash((const char *)x, sizeof(y)) % z

// TODO: add support for ipv6
SEC("xdp")
int udplb(struct xdp_md *ctx) {
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;

    // validate packet length and checks if packet is IP and UDP.
    if (!must_loadbalance(data, data_end, UDPLB_IP, UDPLB_PORT))
        return XDP_PASS;

    struct ethhdr *ethh = data;
    struct iphdr *iph = (struct iphdr *)(ethh + 1);
    struct udphdr *udph = (struct udphdr *)(iph + 1);
    debug_recv(ethh, iph);

    // -------------------------------
    // -- Loadbalancing packet
    // -------------------------------
    // compute modulo
    __u32 key = hash_modulo(iph, struct iphdr, n_backends);

    // get backend spec
    struct backend_spec *backend = bpf_map_lookup_elem(&backends, &key);
    if (!backend || !backend->enabled) {
        bpf_printk("[ERROR] cannot load balance packet: no backend available");
        return XDP_PASS;
    }

    // -------------------------------
    // -- Update packet
    // -------------------------------

    __builtin_memcpy(ethh->h_dest, backend->mac, ETH_ALEN);
    iph->daddr = bpf_htonl(backend->ip);
    udph->dest = bpf_htons(backend->port);

    // -------------------------------
    // -- Update packet
    // -------------------------------

    iph->check = iphdr_csum(iph);
    udph->check = udphdr_csum(udph);

    // -- debug logger
    debug_forw(ethh, iph);

    // XDP_TX:
    // - Return the updated packet to the net iface it came from.
    // - It bypasses normal network stack processing
    return XDP_TX;
}

char _license[] SEC("license") = "GPL";
