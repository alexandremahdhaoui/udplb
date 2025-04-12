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
#include <bpf/bpf_endian.h>
#include <linux/bpf.h>
// clang-format on

#include <bpf/bpf_helpers.h>
#include <linux/if_ether.h>
#include <linux/in.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>

/*******************************************************************************
 * PACKET HELPERS
 *
 ******************************************************************************/

// ["U", "U", "I", "D"] in ASCII.
#define UDPLB_PACKET_PREFIX (0x55 << 24) + (0x55 << 16) + (0x49 << 8) + 0x44

// udpdata holds the udp prefix and session id stored in the udp datagram.
// the prefix must be equal to 0x55554944.
struct __attribute__((packed)) udpdata {
    __u32 prefix;
    __u128 session_id;
};

// must_loadbalance performs the following check:
// - ethhdr bounds
// - ethhdr->h_proto == ETH_P_IP
// - iphdr bounds
// - iphdr->daddr == loadbalancer's IP addr
// - iphdr->protocol == UDP
// - udpdata bounds
static __always_inline _Bool must_loadbalance(void *data, void *data_end,
                                              __u32 lb_addr, __u16 lb_port) {
    // -- Check ethhdr bounds
    // NB: ethh + 1 returns the addr after the sizeof(struct ethhdr)
    struct ethhdr *ethh = data;
    if ((void *)(ethh + 1) > data_end)
        return 0;

    // -- Check if IPV4
    // NB: htons stands for "Host to Network Short"
    if (ethh->h_proto != __constant_htons(ETH_P_IP))
        return 0;

    // -- Check iphhdr bounds
    struct iphdr *iph = (struct iphdr *)(ethh + 1);
    if ((void *)(iph + 1) > data_end)
        return 0;

    // -- Check if dest addr is the loadbalancer's IP.
    if (iph->daddr != bpf_htonl(lb_addr))
        return 0;

    // -- Check if protocol is UDP
    if (iph->protocol != IPPROTO_UDP)
        return 0;

    // -- Check udphdr bounds
    struct udphdr *udph = (struct udphdr *)(iph + 1);
    if ((void *)(udph + 1) > data_end)
        return 0;

    // -- Check if dest addr is the loadbalancer's IP.
    if (udph->dest != bpf_htons(lb_port)) {
        return 0;
    }

    // -- Check udpdata
    // The udp data is prefixed by a 4 byte pattern equal to [0x55, 0x55, 0x49,
    // 0x44] followed by 16 bytes uuid.
    struct udpdata *udpd = (struct udpdata *)(udph + 1);
    if ((void *)(udpd + 1) > data_end)
        return 0;

    if (udpd->prefix != UDPLB_PACKET_PREFIX)
        return 0;

    return 1;
}

// ---------------------------------------------------------------------------
// -- CHECKSUM
// ---------------------------------------------------------------------------

// computes a __u16 checksum.
static __always_inline __u16 csum(__u32 *hdr, __u32 size) {
    __s64 csum = bpf_csum_diff(0, 0, hdr, size, 0);

    // Ensures the carry bit is added back to the sum.
    // It masks the lower 16 bits & add the carry bit.
    // We call it twice because we can't use a while loop
    // because of the bpf verifier.
    for (int i = 0; i < 4; i++) {
        csum = (csum & 0xffff) + (csum >> 16);
    }

    return csum;
}

// computes the __u16 checksum of the iphdr.
static __always_inline __u16 iphdr_csum(struct iphdr *iph) {
    iph->check = 0;
    return csum((__u32 *)iph, sizeof(struct iphdr));
}

// computes the __u16 checksum of the udphdr.
static __always_inline __u16 udphdr_csum(struct udphdr *udph) {
    udph->check = 0;
    return csum((__u32 *)udph, sizeof(struct udphdr));
}

// ---------------------------------------------------------------------------
// -- FAST HASH
// ---------------------------------------------------------------------------

// computes the hash of x of size y and return its z modulo
#define hash_modulo(x, y, z) fast_hash((const char *)x, sizeof(y)) % z

// computes a fast hash
// TODO: benchmark this func w/ other hash funcs.
static __always_inline __u32 fast_hash(const char *str, __u32 len) {
    __u32 hash = 0;
    for (int i = 0; i < len; i++)
        hash = (*str++) + (hash << 6) + (hash << 16) - hash;

    return hash;
}

// ---------------------------------------------------------------------------
// -- DEBUG
// ---------------------------------------------------------------------------

static __always_inline void debug_recv(struct ethhdr *ethh, struct iphdr *iph) {
    __u32 daddr = bpf_ntohl(iph->daddr);
    __u32 saddr = bpf_ntohl(iph->saddr);

    bpf_printk("[RECV] saddr: %d.%d.%d.%d", //
               (saddr >> 24) & 0xFF, (saddr >> 16) & 0xFF, (saddr >> 8) & 0xFF,
               (saddr) & 0xFF);

    bpf_printk("[RECV] daddr: %d.%d.%d.%d", //
               (daddr >> 24) & 0xFF, (daddr >> 16) & 0xFF, (daddr >> 8) & 0xFF,
               (daddr) & 0xFF);

    bpf_printk("[RECV] h_source: %x:%x:%x:%x:%x:%x", ethh->h_source[0],
               ethh->h_source[1], ethh->h_source[2], ethh->h_source[3],
               ethh->h_source[4], ethh->h_source[5]);

    bpf_printk("[RECV] h_dest: %x:%x:%x:%x:%x:%x", ethh->h_dest[0],
               ethh->h_dest[1], ethh->h_dest[2], ethh->h_dest[3],
               ethh->h_dest[4], ethh->h_dest[5]);
}

static __always_inline void debug_forw(struct ethhdr *ethh, struct iphdr *iph) {
    __u32 daddr = bpf_ntohl(iph->daddr);
    __u32 saddr = bpf_ntohl(iph->saddr);

    bpf_printk("[SEND] saddr: %d.%d.%d.%d", //
               (saddr >> 24) & 0xFF, (saddr >> 16) & 0xFF, (saddr >> 8) & 0xFF,
               (saddr) & 0xFF);

    bpf_printk("[SEND] daddr: %d.%d.%d.%d", //
               (daddr >> 24) & 0xFF, (daddr >> 16) & 0xFF, (daddr >> 8) & 0xFF,
               (daddr) & 0xFF);

    bpf_printk("[SEND] h_dest: %x:%x:%x:%x:%x:%x", ethh->h_dest[0],
               ethh->h_dest[1], ethh->h_dest[2], ethh->h_dest[3],
               ethh->h_dest[4], ethh->h_dest[5]);
}
