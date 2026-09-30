//go:build ignore

// netmon.c: observe outbound TCP connections without touching the monitored services.
//
// One tracepoint (sock:inet_sock_set_state) is enough:
//   * ->SYN_SENT (process context inside connect()): remember which PID/comm is connecting
//   * SYN_SENT -> ESTABLISHED : CONNECT        (handshake succeeded)
//   * SYN_SENT -> CLOSE       : CONNECT_FAILED (refused / timed out)
//   * ->CLOSE for a connection we saw established: CLOSE (with lifetime)
//
// The tracepoint context is declared by hand (layout from
// /sys/kernel/tracing/events/sock/inet_sock_set_state/format) so no vmlinux.h / CO-RE is needed.
// Only IPv4 TCP is handled. `struct event` MUST match internal/raw/raw.go (56 bytes).

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>

char LICENSE[] SEC("license") = "GPL";

#define AF_INET 2
#define IPPROTO_TCP 6
#define TCP_ESTABLISHED 1
#define TCP_SYN_SENT 2
#define TCP_CLOSE 7

#define KIND_CONNECT 1
#define KIND_CONNECT_FAILED 2
#define KIND_CLOSE 3

struct set_state_ctx {
	__u16 common_type;
	__u8 common_flags;
	__u8 common_preempt_count;
	__s32 common_pid;

	const void *skaddr;
	__s32 oldstate;
	__s32 newstate;
	__u16 sport; // host byte order
	__u16 dport; // host byte order
	__u16 family;
	__u16 protocol;
	__u8 saddr[4];
	__u8 daddr[4];
	__u8 saddr_v6[16];
	__u8 daddr_v6[16];
};

struct sock_val {
	__u64 ts;
	__u32 pid;
	char comm[16];
	__u32 pad;
};

struct event {
	__u64 ts_ns;
	__u64 duration_ns;
	__u32 pid;
	__u32 kind;
	__u8 saddr[4];
	__u8 daddr[4];
	__u16 sport;
	__u16 dport;
	char comm[16];
	__u32 pad;
};

// connect() in flight, keyed by struct sock *
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, 16384);
	__type(key, __u64);
	__type(value, struct sock_val);
} pending SEC(".maps");

// established client connections, keyed by struct sock *
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, 16384);
	__type(key, __u64);
	__type(value, struct sock_val);
} conns SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 18);
} events SEC(".maps");

// Force `struct event` into the object's BTF.
const struct event *unused_event __attribute__((unused));

static __always_inline void emit(struct set_state_ctx *ctx, const struct sock_val *v,
				 __u32 kind, __u64 now, __u64 duration)
{
	struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
	if (!e)
		return;
	e->ts_ns = now;
	e->duration_ns = duration;
	e->pid = v->pid;
	e->kind = kind;
	__builtin_memcpy(e->saddr, ctx->saddr, 4);
	__builtin_memcpy(e->daddr, ctx->daddr, 4);
	e->sport = ctx->sport;
	e->dport = ctx->dport;
	__builtin_memcpy(e->comm, v->comm, 16);
	e->pad = 0;
	bpf_ringbuf_submit(e, 0);
}

SEC("tracepoint/sock/inet_sock_set_state")
int handle_set_state(struct set_state_ctx *ctx)
{
	if (ctx->protocol != IPPROTO_TCP || ctx->family != AF_INET)
		return 0;

	__u64 sk = (__u64)(unsigned long)ctx->skaddr;
	int oldstate = ctx->oldstate;
	int newstate = ctx->newstate;
	__u64 now = bpf_ktime_get_ns();

	if (newstate == TCP_SYN_SENT) {
		// Runs in the context of the process calling connect(): capture who it is.
		// (Ports/addresses are not final yet at this point, so they are read later.)
		struct sock_val v = {};
		v.ts = now;
		v.pid = bpf_get_current_pid_tgid() >> 32;
		bpf_get_current_comm(&v.comm, sizeof(v.comm));
		bpf_map_update_elem(&pending, &sk, &v, BPF_ANY);
		return 0;
	}

	if (oldstate == TCP_SYN_SENT && newstate == TCP_ESTABLISHED) {
		struct sock_val *p = bpf_map_lookup_elem(&pending, &sk);
		if (!p)
			return 0;
		struct sock_val c = {};
		c.ts = now;
		c.pid = p->pid;
		__builtin_memcpy(c.comm, p->comm, 16);
		bpf_map_update_elem(&conns, &sk, &c, BPF_ANY);
		emit(ctx, p, KIND_CONNECT, now, now - p->ts);
		bpf_map_delete_elem(&pending, &sk);
		return 0;
	}

	if (oldstate == TCP_SYN_SENT && newstate == TCP_CLOSE) {
		struct sock_val *p = bpf_map_lookup_elem(&pending, &sk);
		if (!p)
			return 0;
		emit(ctx, p, KIND_CONNECT_FAILED, now, now - p->ts);
		bpf_map_delete_elem(&pending, &sk);
		return 0;
	}

	if (newstate == TCP_CLOSE) {
		struct sock_val *c = bpf_map_lookup_elem(&conns, &sk);
		if (!c)
			return 0;
		emit(ctx, c, KIND_CLOSE, now, now - c->ts);
		bpf_map_delete_elem(&conns, &sk);
		return 0;
	}

	return 0;
}
