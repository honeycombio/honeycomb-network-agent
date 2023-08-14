// go:build ignore

#include "vmlinux.h"
#include "bpf_endian.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"

char __license[] SEC("license") = "Dual MIT/GPL";

typedef struct tcp_event
{
	u64 start_time;
	u64 end_time;
	u32 daddr;
	u16 dport;
	u32 saddr;
	u16 sport;
} tcp_event;

struct
{
	__uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
} events SEC(".maps");

struct
{
	__uint(type, BPF_MAP_TYPE_HASH);
	__type(key, u64);
	__type(value, struct tcp_event);
	__uint(max_entries, 1024);
} context_to_http_events SEC(".maps");

SEC("kprobe/tcp_connect")
int kprobe__tcp_connect(struct pt_regs *ctx)
{
	u64 pid = bpf_get_current_pid_tgid();
	tcp_event event = {};
	event.start_time = bpf_ktime_get_ns();

	struct sock *sock = (struct sock *)PT_REGS_PARM1(ctx);
	bpf_probe_read(&event.daddr, sizeof(event.daddr), &sock->__sk_common.skc_daddr);
	bpf_probe_read(&event.dport, sizeof(event.dport), &sock->__sk_common.skc_dport);
	bpf_probe_read(&event.saddr, sizeof(event.saddr), &sock->__sk_common.skc_rcv_saddr);

	u16 sport = 0;
	bpf_probe_read(&sport, sizeof(event.sport), &sock->__sk_common.skc_num);
	event.sport = bpf_ntohs(sport);

	bpf_map_update_elem(&context_to_http_events, &pid, &event, BPF_ANY);
	return 0;
}

SEC("kprobe/tcp_close")
int kprobe__tcp_close(struct pt_regs *ctx)
{
	u64 pid = bpf_get_current_pid_tgid();
	void *event_ptr = bpf_map_lookup_elem(&context_to_http_events, &pid);
	if (!event_ptr)
	{
		return 0;
	}

	struct tcp_event event = {};
	bpf_probe_read(&event, sizeof(tcp_event), event_ptr);
	event.end_time = bpf_ktime_get_ns();

	bpf_perf_event_output((void *)ctx, &events, BPF_F_CURRENT_CPU, &event, sizeof(tcp_event));
	bpf_map_delete_elem(&context_to_http_events, &pid);
	return 0;
}