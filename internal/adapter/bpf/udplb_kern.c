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

typedef struct {
    // The loadbalancer's IP is checked to decide if a packet must be
    // loadbalanced. The config.ip endianness must be in the host's.
    __u32 ip;
    // The loadbalancer's port is checked to decide if a packet must be
    // loadbalanced.
    __u16 port;
    // The size of the lookup table.
    __u32 lookup_table_size;
} config_t;

// The config is a const. If users wants to update the loadbalancer's config
// they must rollout a new deployment, or start a new process specifying the
// new config.
volatile const config_t config;

/*******************************************************************************
 * active_pointer
 *
 *
 ******************************************************************************/

// This is a variable that can be equal to 0 or 1.
//  - When equal to 0, the `*_a` maps and `*_a_len` variables are active, and
//    the bpf program will read from these variants.
//  - When equal to 1, the `*_b` maps and `*_b_len` variables are active, and
//    the bpf program will read from these variants.
volatile __u8 active_pointer;

/*******************************************************************************
 * backends
 *
 * This section defines:
 * - backend_spec_t type.
 * - backends_{a,b} maps.
 * - backends_{a,b}_len variables.
 ******************************************************************************/

#define BACKEND_INDEX __u32
#define BACKEND_ID_TYPE __u128

typedef struct {
    BACKEND_ID_TYPE id;
    __u32 ip;
    __u16 port;
    unsigned char mac[ETH_ALEN];
} backend_spec_t;

// The set of all backends.
typedef struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 64); // Max 64 backends (can be changed).
    __type(key, BACKEND_INDEX);
    __type(value, backend_spec_t);
} backends_t;

backends_t backends_a SEC(".maps");
backends_t backends_b SEC(".maps");

volatile __u32 backends_a_len;
volatile __u32 backends_b_len;

static __always_inline backends_t *get_active_backends() {
    if (active_pointer) {
        return &backends_a;
    }
    return &backends_b;
}

/*******************************************************************************
 * lookup_table
 *
 * This section defines:
 * - lookup_table_{a,b} maps.
 * - lookup_table_{a,b}_len variables.
 ******************************************************************************/

#define LOOKUP_TABLE_MAX_LENGTH 1 << 16
#define LOOKUP_TABLE_INDEX __u32

typedef struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, LOOKUP_TABLE_MAX_LENGTH);
    __type(key, LOOKUP_TABLE_INDEX);
    __type(value, BACKEND_INDEX);
} lookup_table_t;

lookup_table_t lookup_table_a SEC(".maps");
lookup_table_t lookup_table_b SEC(".maps");

volatile __u32 lookup_table_a_len;
volatile __u32 lookup_table_b_len;

static __always_inline lookup_table_t *get_active_lookup_table() {
    if (active_pointer) {
        return &lookup_table_a;
    }
    return &lookup_table_b;
}

/*******************************************************************************
 * sessions
 *
 * This section defines:
 * - sessions_{a,b} maps.
 * - sessions_{a,b}_len variables.
 * - session_assignment_t type.
 * - sessions_fifo queue.
 ******************************************************************************/

#define SESSIONS_MAX_LENGTH 1 << 16 // 65,536
#define SESSION_ID_TYPE __u128
#define SESSIONS_FIFO_MAX_LENGTH 1 << 8

typedef struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, SESSIONS_MAX_LENGTH);
    __type(key, SESSION_ID_TYPE);
    __type(value, BACKEND_INDEX);
} sessions_t;

sessions_t sessions_a SEC(".maps");
sessions_t sessions_b SEC(".maps");

volatile __u32 sessions_a_len;
volatile __u32 sessions_b_len;

static __always_inline sessions_t *get_active_sessions() {
    if (active_pointer) {
        return &sessions_a;
    }
    return &sessions_b;
}

// session_assignment_t assigns a session to a backend. Both entity are
// identified by their id of type __u128 (uuidv4).
typedef struct {
    SESSION_ID_TYPE session_id;
    BACKEND_ID_TYPE backend_id;
} session_assignment_t;

typedef struct {
    __uint(type, BPF_MAP_TYPE_QUEUE);
    __uint(max_entries, SESSIONS_FIFO_MAX_LENGTH);
    __type(value, session_assignment_t);
} sessions_fifo_t;

sessions_fifo_t sessions_fifo SEC(".maps");

/*******************************************************************************
 * udplb
 *
 ******************************************************************************/

// TODO: add support for ipv6
SEC("xdp") int udplb(struct xdp_md *ctx) {
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;

    // validate packet length and checks if packet is IP and UDP.
    if (!must_loadbalance(data, data_end, config.ip, config.port))
        return XDP_PASS;

    struct ethhdr *ethh = data;
    struct iphdr *iph = (struct iphdr *)(ethh + 1);
    struct udphdr *udph = (struct udphdr *)(iph + 1);
    struct udpdata *udpd = (struct udpdata *)(udph + 1);
    debug_recv(ethh, iph);

    // -------------------------------
    // -- Loadbalancing packet
    // -------------------------------

    backends_t *backends = get_active_backends();
    lookup_table_t *lup = get_active_lookup_table();
    sessions_t *sess = get_active_sessions();

    // -- compute modulo
    __u32 key = hash_modulo(iph, struct iphdr, config.lookup_table_size);

    // -- backend index from sessions
    __u8 new_session = 0; // false
    __u32 *backend_idx = bpf_map_lookup_elem(sess, &key);
    if (!backend_idx) {
        // -- backend index from lookup table
        new_session = 1; // true
        backend_idx = bpf_map_lookup_elem(lup, &key);
        if (!backend_idx) { // return if no backend idx
            bpf_printk("[ERROR] cannot load balance packet: no backend");
            return XDP_PASS;
        }
    }

    // -- backend spec
    backend_spec_t *backend = bpf_map_lookup_elem(backends, backend_idx);
    if (!backend) {
        bpf_printk("[ERROR] cannot load balance packet: no backend available");
        return XDP_PASS;
    }

    // persist the hash_modulo to backend_idx mapping.
    if (new_session) {
        session_assignment_t assignment = {
            .backend_id = backend->id,
            .session_id = udpd->session_id,
        };

        long err = bpf_map_push_elem(&sessions_fifo, &assignment, BPF_ANY);
        err = bpf_map_update_elem(sess, &key, backend_idx, BPF_ANY);
        if (err < 0)
            bpf_printk("[ERROR] unable to map session");
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
