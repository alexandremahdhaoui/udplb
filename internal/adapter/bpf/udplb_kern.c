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
 * - backend_list_{a,b} maps.
 * - backend_list_{a,b}_len variables.
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
} backend_list_t;

backend_list_t backend_list_a SEC(".maps");
backend_list_t backend_list_b SEC(".maps");

volatile __u32 backend_list_a_len;
volatile __u32 backend_list_b_len;

static __always_inline backend_list_t *get_active_backend_list() {
    switch (active_pointer) {
    case 0:
        return &backend_list_a;
    default:
        return &backend_list_b;
    }
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
 * - session_map_{a,b} maps.
 * - session_map_{a,b}_len variables.
 ******************************************************************************/

#define SESSIONS_MAX_LENGTH 1 << 16 // 65,536
#define SESSION_ID_TYPE __u128

typedef struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, SESSIONS_MAX_LENGTH);
    __type(key, SESSION_ID_TYPE);
    __type(value, BACKEND_INDEX);
} session_map_t;

session_map_t session_map_a SEC(".maps");
session_map_t session_map_b SEC(".maps");

volatile __u32 session_map_a_len;
volatile __u32 session_map_b_len;

static __always_inline session_map_t *get_active_sessions() {
    if (active_pointer) {
        return &session_map_a;
    }
    return &session_map_b;
}

/*******************************************************************************
 * assignments
 *
 * This section defines:
 * - assignment_t type.
 * - assignment_ringbuf ring buffer.
 *
 * Links:
 * - https://docs.ebpf.io/linux/map-type/BPF_MAP_TYPE_RINGBUF/
 * - https://docs.ebpf.io/linux/helper-function/bpf_ringbuf_submit/
 * - https://nakryiko.com/posts/bpf-ringbuf/#bpf-ringbuf-bpf-ringbuf-output
 ******************************************************************************/

#define ASSIGNMENT_RINGBUF_SIZE sizeof(assignment_t) * 1024 // 36KB

// session_assignment_t assigns a session to a backend. Both entity are
// identified by their id of type __u128 (uuidv4).
typedef struct {
    SESSION_ID_TYPE session_id;
    BACKEND_ID_TYPE backend_id;
} assignment_t;

typedef struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, ASSIGNMENT_RINGBUF_SIZE);
    __type(value, assignment_t);
} assignment_ringbuf_t;

assignment_ringbuf_t assignment_ringbuf SEC(".maps");

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

    backend_list_t *backends = get_active_backend_list();
    lookup_table_t *lup = get_active_lookup_table();
    session_map_t *sess = get_active_sessions();

    // -- compute key from session id & get backend index from session map.
    __u32 key = hash_modulo(udpd->session_id, __u128, config.lookup_table_size);
    __u32 *backend_idx = bpf_map_lookup_elem(sess, &key);
    __u8 new_session = (backend_idx == NULL);

    // -- get idx from lookup table if new session
    if (new_session) {
        backend_idx = bpf_map_lookup_elem(lup, &key);
        if (backend_idx == NULL) {
            bpf_printk(
                "[ERROR] cannot load balance packet: lookup table error");
            return XDP_PASS;
        }
    }

    // -- backend spec
    backend_spec_t *backend = bpf_map_lookup_elem(backends, backend_idx);
    if (backend == NULL) {
        bpf_printk("[ERROR] cannot load balance packet: no backend found");
        return XDP_PASS;
    }

    // persist assignment if new session
    if (new_session) {
        assignment_t *a;

        // "notify" userland about this new assignment.
        long err = bpf_map_push_elem(&assignment_ringbuf, &a, BPF_ANY);
        if (err < 0) // Failing is fine
            bpf_printk("[ERROR] unable to add new session to fifo");

        a = bpf_ringbuf_reserve(&assignment_ringbuf, sizeof(assignment_t), 0);
        if (a != NULL) {
            a->backend_id = backend->id;
            a->session_id = udpd->session_id;
            bpf_ringbuf_submit(a, 0);
        } else {
            bpf_printk("[ERROR] unable to write new assignment to ring buffer");
        }

        // persist "locally" (i.e. in the active bpf map)
        err = bpf_map_update_elem(sess, &key, backend_idx, BPF_ANY);
        if (err < 0) // Failing is fine.
            bpf_printk("[ERROR] unable to map new session");
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
