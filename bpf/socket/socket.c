// go:build ignore

#include "vmlinux.h"
#include "bpf_endian.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"

char __license[] SEC("license") = "Dual MIT/GPL";

struct sock_common
{
    union
    {
        struct
        {
            __be32 skc_daddr;
            __be32 skc_rcv_saddr;
        };
    };
    union
    {
        // Padding out union skc_hash.
        __u32 _;
    };
    union
    {
        struct
        {
            __be16 skc_dport;
            __u16 skc_num;
        };
    };
    short unsigned int skc_family;
};

struct sock
{
    struct sock_common __sk_common;
};

typedef struct http_event_t
{
    u64 start_time;
    u64 end_time;
    u32 daddr;
    u16 dport;
    u32 saddr;
    u16 sport;
    u64 bytes_sent;
} http_event_t;

struct
{
    __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
} events SEC(".maps");

SEC("socket/http_filter")
int socket__http_filter(struct __sk_buff *skb)
{
    http_event_t event = {};
    event.start_time = bpf_ktime_get_ns();

    bpf_perf_event_output(skb, &events, BPF_F_CURRENT_CPU, &event, sizeof(http_event_t));
    return 0;
}

struct sk_buff
{
    union
    {
        struct
        {
            struct sk_buff *next;
            struct sk_buff *prev;
            union
            {
                struct net_device *dev;
                unsigned long dev_scratch;
            };
        };
        struct rb_node rbnode;
        struct list_head list;
        struct llist_node ll_node;
    };
    union
    {
        struct sock *sk;
        int ip_defrag_offset;
    };
    union
    {
        ktime_t tstamp;
        u64 skb_mstamp_ns;
    };
    char cb[48];
    union
    {
        struct
        {
            unsigned long _skb_refdst;
            void (*destructor)(struct sk_buff *skb);
        };
        struct list_head tcp_tsorted_anchor;
#ifdef CONFIG_NET_SOCK_MSG;
        unsigned long _sk_redir;
#endif;
    };
#if defined(CONFIG_NF_CONNTRACK) || defined(CONFIG_NF_CONNTRACK_MODULE);
    unsigned long _nfct;
#endif;
    unsigned int len, data_len;
    __u16 mac_len, hdr_len;
    __u16 queue_mapping;
#ifdef __BIG_ENDIAN_BITFIELD;
#define CLONED_MASK (1 << 7);
#else;
#define CLONED_MASK 1;
#endif;
#define CLONED_OFFSET offsetof(struct sk_buff, __cloned_offset);
    __u8 cloned : 1, nohdr : 1, fclone : 2, peeked : 1, head_frag : 1, pfmemalloc : 1, pp_recycle : 1;
#ifdef CONFIG_SKB_EXTENSIONS;
    __u8 active_extensions;
#endif;
    __u8 pkt_type : 3;
    __u8 ignore_df : 1;
    __u8 dst_pending_confirm : 1;
    __u8 ip_summed : 2;
    __u8 ooo_okay : 1;
    __u8 mono_delivery_time : 1;
#ifdef CONFIG_NET_CLS_ACT;
    __u8 tc_at_ingress : 1;
    __u8 tc_skip_classify : 1;
#endif;
    __u8 remcsum_offload : 1;
    __u8 csum_complete_sw : 1;
    __u8 csum_level : 2;
    __u8 inner_protocol_type : 1;
    __u8 l4_hash : 1;
    __u8 sw_hash : 1;
#ifdef CONFIG_WIRELESS;
    __u8 wifi_acked_valid : 1;
    __u8 wifi_acked : 1;
#endif;
    __u8 no_fcs : 1;
    __u8 encapsulation : 1;
    __u8 encap_hdr_csum : 1;
    __u8 csum_valid : 1;
#ifdef CONFIG_IPV6_NDISC_NODETYPE;
    __u8 ndisc_nodetype : 2;
#endif;
#if IS_ENABLED(CONFIG_IP_VS);
    __u8 ipvs_property : 1;
#endif;
#if IS_ENABLED(CONFIG_NETFILTER_XT_TARGET_TRACE) || IS_ENABLED(CONFIG_NF_TABLES);
    __u8 nf_trace : 1;
#endif;
#ifdef CONFIG_NET_SWITCHDEV;
    __u8 offload_fwd_mark : 1;
    __u8 offload_l3_fwd_mark : 1;
#endif;
    __u8 redirected : 1;
#ifdef CONFIG_NET_REDIRECT;
    __u8 from_ingress : 1;
#endif;
#ifdef CONFIG_NETFILTER_SKIP_EGRESS;
    __u8 nf_skip_egress : 1;
#endif;
#ifdef CONFIG_TLS_DEVICE;
    __u8 decrypted : 1;
#endif;
    __u8 slow_gro : 1;
#if IS_ENABLED(CONFIG_IP_SCTP);
    __u8 csum_not_inet : 1;
#endif;
#ifdef CONFIG_NET_SCHED;
    __u16 tc_index;
#endif;
    u16 alloc_cpu;
    union
    {
        __wsum csum;
        struct
        {
            __u16 csum_start;
            __u16 csum_offset;
        };
    };
    __u32 priority;
    int skb_iif;
    __u32 hash;
    union
    {
        u32 vlan_all;
        struct
        {
            __be16 vlan_proto;
            __u16 vlan_tci;
        };
    };
#if defined(CONFIG_NET_RX_BUSY_POLL) || defined(CONFIG_XPS);
    union
    {
        unsigned int napi_id;
        unsigned int sender_cpu;
    };
#endif;
#ifdef CONFIG_NETWORK_SECMARK;
    __u32 secmark;
#endif;
    union
    {
        __u32 mark;
        __u32 reserved_tailroom;
    };
    union
    {
        __be16 inner_protocol;
        __u8 inner_ipproto;
    };
    __u16 inner_transport_header;
    __u16 inner_network_header;
    __u16 inner_mac_header;
    __be16 protocol;
    __u16 transport_header;
    __u16 network_header;
    __u16 mac_header;
#ifdef CONFIG_KCOV;
    u64 kcov_handle;
#endif;
    sk_buff_data_t tail;
    sk_buff_data_t end;
    unsigned char *head, *data;
    unsigned int truesize;
    refcount_t users;
#ifdef CONFIG_SKB_EXTENSIONS;
    struct skb_ext *extensions;
#endif;
};