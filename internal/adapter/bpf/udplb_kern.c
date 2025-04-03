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

/*******************************************************************************
 * config_t
 *
 *
 ******************************************************************************/

struct config_t {
    // The loadbalancer's IP is checked to decide if a packet must be
    // loadbalanced. The config.ip endianness must be in the host's.
    __u32 ip;
    // The loadbalancer's port is checked to decide if a packet must be
    // loadbalanced.
    __u16 port;
    // The size of the lookup table.
    __u32 lookup_table_size;
};

// The config is a const. If users wants to update the loadbalancer's config
// they must rollout a new deployment, or start a new process specifying the
// new config.
volatile const struct config_t config;

/*******************************************************************************
 * active_pointer
 *
 *
 ******************************************************************************/

// This is a variable that can be equal to 0 or 1.
//  - When equal to 0, the `*_a` maps and `*_a_len` variables are active, and
//    the bpf program will read from these variants.
//  - When equal to 1, the `*_b` maps and
volatile __u8 active_pointer;

/*******************************************************************************
 * backends
 *
 * This section defines:
 * - backend_spec
 * - a/b backend maps.
 * - {a,b}_len variables.
 ******************************************************************************/

struct backend_spec {
    __u32 ip;
    __u16 port;
    unsigned char mac[ETH_ALEN];
    __u8 enabled;
};

// The set of all backends.
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 64); // Max 64 backends.
    __type(key, __u32);
    __type(value, struct backend_spec);
} backends_a SEC(".maps");

// The set of all backends.
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 64); // Max 64 backends.
    __type(key, __u32);
    __type(value, struct backend_spec);
} backends_b SEC(".maps");

volatile __u32 backends_a_len;
volatile __u32 backends_b_len;

/*******************************************************************************
 * lookup_table
 *
 * This section defines:
 * - lookup_table_{a,b} maps.
 * - lookup_table_{a,b}_len variables.
 ******************************************************************************/

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, ((1 << 16) - 1));
    __type(key, __u32);
    __type(value, __u32);
} lookup_table_a SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, ((1 << 16) - 1));
    __type(key, __u32);
    __type(value, __u32);
} lookup_table_b SEC(".maps");

volatile __u32 lookup_table_a_len;
volatile __u32 lookup_table_b_len;

/*******************************************************************************
 * udplb
 *
 ******************************************************************************/

// TODO: add support for ipv6
SEC("xdp")
int udplb(struct xdp_md *ctx) {
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;

    // validate packet length and checks if packet is IP and UDP.
    if (!must_loadbalance(data, data_end, config.ip, config.port))
        return XDP_PASS;

    struct ethhdr *ethh = data;
    struct iphdr *iph = (struct iphdr *)(ethh + 1);
    struct udphdr *udph = (struct udphdr *)(iph + 1);
    debug_recv(ethh, iph);

    // -------------------------------
    // -- Loadbalancing packet
    // -------------------------------
    // compute modulo
    __u32 key = hash_modulo(iph, struct iphdr, config.lookup_table_size);

    // TODO: make use of A and B lookup_table/backends.

    // get backend spec
    struct backend_spec *backend = bpf_map_lookup_elem(&backends_a, &key);
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
