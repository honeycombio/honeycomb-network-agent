// go:build ignore

#include "vmlinux.h"
#include "bpf_endian.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"

char __license[] SEC("license") = "Dual MIT/GPL";

// example based off https://github.com/iovisor/bcc/blob/master/examples/networking/http_filter/http-parse-simple.c

#define cursor_advance(_cursor, _len) \
    ({ void *_tmp = _cursor; _cursor += _len; _tmp; })
#define IP_TCP 6
#define ETH_HLEN 14
#define MIN_HTTP_SIZE 12

struct ethernet_t
{
    unsigned long long dst : 48;
    unsigned long long src : 48;
    unsigned int type : 16;
};

struct ip_t
{
    unsigned char ver : 4; // byte 0
    unsigned char hlen : 4;
    unsigned char tos;
    unsigned short tlen;
    unsigned short identification; // byte 4
    unsigned short ffo_unused : 1;
    unsigned short df : 1;
    unsigned short mf : 1;
    unsigned short foffset : 13;
    unsigned char ttl; // byte 8
    unsigned char nextp;
    unsigned short hchecksum;
    unsigned int src; // byte 12
    unsigned int dst; // byte 16
};

struct tcp_t
{
    unsigned short src_port; // byte 0
    unsigned short dst_port;
    unsigned int seq_num;     // byte 4
    unsigned int ack_num;     // byte 8
    unsigned char offset : 4; // byte 12
    unsigned char reserved : 4;
    unsigned char flag_cwr : 1;
    unsigned char flag_ece : 1;
    unsigned char flag_urg : 1;
    unsigned char flag_ack : 1;
    unsigned char flag_psh : 1;
    unsigned char flag_rst : 1;
    unsigned char flag_syn : 1;
    unsigned char flag_fin : 1;
    unsigned short rcv_wnd;
    unsigned short cksum; // byte 16
    unsigned short urg_ptr;
};

struct http_event_t
{
    u64 start_time;
    u64 end_time;
    u32 daddr;
    u16 dport;
    u32 saddr;
    u16 sport;
    u64 bytes_sent;
};

struct
{
    __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
} events SEC(".maps");

SEC("socket/http_filter")
int socket__http_filter(struct __sk_buff *skb)
{
    u8 *cursor = 0;

    struct ethernet_t *ethernet = cursor_advance(cursor, sizeof(*ethernet));
    // filter IP packets (ethernet type = 0x0800)
    if (!(ethernet->type == 0x0800))
    {
        goto DROP;
    }

    struct ip_t *ip = cursor_advance(cursor, sizeof(*ip));
    // filter TCP packets (ip next protocol = 0x06)
    if (ip->nextp != IP_TCP)
    {
        goto DROP;
    }

    u32 tcp_header_length = 0;
    u32 ip_header_length = 0;
    u32 payload_offset = 0;
    u32 payload_length = 0;

    // calculate ip header length
    // value to multiply * 4
    // e.g. ip->hlen = 5 ; IP Header Length = 5 x 4 byte = 20 byte
    ip_header_length = ip->hlen << 2; // SHL 2 -> *4 multiply

    // check ip header length against minimum
    if (ip_header_length < sizeof(*ip))
    {
        goto DROP;
    }

    // shift cursor forward for dynamic ip header size
    void *_ = cursor_advance(cursor, (ip_header_length - sizeof(*ip)));

    struct tcp_t *tcp = cursor_advance(cursor, sizeof(*tcp));

    // calculate tcp header length
    // value to multiply *4
    // e.g. tcp->offset = 5 ; TCP Header Length = 5 x 4 byte = 20 byte
    tcp_header_length = tcp->offset << 2; // SHL 2 -> *4 multiply

    // calculate payload offset and length
    payload_offset = ETH_HLEN + ip_header_length + tcp_header_length;
    payload_length = ip->tlen - ip_header_length - tcp_header_length;

    // http://stackoverflow.com/questions/25047905/http-request-minimum-size-in-bytes
    // minimum length of http request is always geater than 7 bytes
    // avoid invalid access memory
    // include empty payload
    if (payload_length < 7)
    {
        goto DROP;
    }

    // load first 7 byte of payload into p (payload_array)
    // direct access to skb not allowed
    // unsigned long p[7];
    // int i = 0;
    // for (i = 0; i < 7; i++)
    // {
    //     p[i] = load_byte(skb, payload_offset + i);
    // }
    char p[MIN_HTTP_SIZE];
    bpf_skb_load_bytes(skb, payload_offset, p, sizeof(p));

    // find a match with an HTTP message
    // HTTP
    if ((p[0] == 'H') && (p[1] == 'T') && (p[2] == 'T') && (p[3] == 'P'))
    {
        goto KEEP;
    }
    // GET
    if ((p[0] == 'G') && (p[1] == 'E') && (p[2] == 'T'))
    {
        goto KEEP;
    }
    // POST
    if ((p[0] == 'P') && (p[1] == 'O') && (p[2] == 'S') && (p[3] == 'T'))
    {
        goto KEEP;
    }
    // PUT
    if ((p[0] == 'P') && (p[1] == 'U') && (p[2] == 'T'))
    {
        goto KEEP;
    }
    // DELETE
    if ((p[0] == 'D') && (p[1] == 'E') && (p[2] == 'L') && (p[3] == 'E') && (p[4] == 'T') && (p[5] == 'E'))
    {
        goto KEEP;
    }
    // HEAD
    if ((p[0] == 'H') && (p[1] == 'E') && (p[2] == 'A') && (p[3] == 'D'))
    {
        goto KEEP;
    }

    // no HTTP match
    goto DROP;

// keep the packet and send it to userspace returning -1
KEEP:
    return -1;

// drop the packet returning 0
DROP:
    return 0;
}
