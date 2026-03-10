// SPDX-License-Identifier: GPL-2.0
#include <linux/bpf.h>
#include <linux/types.h>
#include <bpf/bpf_helpers.h>

/*
 * Tracepoint context layout for syscalls/sys_enter_openat on 64-bit Linux.
 *
 * From /sys/kernel/debug/tracing/events/syscalls/sys_enter_openat/format:
 *   field:unsigned short common_type;       offset:0;  size:2;
 *   field:unsigned char  common_flags;      offset:2;  size:1;
 *   field:unsigned char  common_preempt_count; offset:3; size:1;
 *   field:int            common_pid;        offset:4;  size:4;
 *   field:int            __syscall_nr;      offset:8;  size:4;
 *   -- 4 bytes implicit padding at offset 12 --
 *   field:unsigned long  dfd;               offset:16; size:8;  <- args[0]
 *   field:const char *   filename;          offset:24; size:8;  <- args[1]
 *   field:int            flags;             offset:32; size:8;  <- args[2]
 *   field:umode_t        mode;              offset:40; size:8;  <- args[3]
 */
struct sys_enter_openat_ctx
{
    __u16 common_type;
    __u8 common_flags;
    __u8 common_preempt_count;
    __s32 common_pid;
    __s32 __syscall_nr;
    __u32 __pad;           /* explicit 4-byte padding before args */
    unsigned long args[6]; /* args[0]=dfd, args[1]=filename ptr, ... */
};

/* Event struct – layout must match the Go counterpart exactly:
 *   offset  0 : pid      (__u32,  4 bytes)
 *   offset  4 : comm     (char[], 16 bytes)
 *   offset 20 : filename (char[], 256 bytes)
 *   offset 276: flags    (__u32,  4 bytes)  – openat flags (O_RDONLY etc.)
 *   total     : 280 bytes
 */
struct event
{
    __u32 pid;
    char comm[16];
    char filename[256];
    __u32 flags;
};

/* Perf event array map used to stream events to user space. */
struct
{
    __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
    __uint(max_entries, 128);
    __uint(key_size, sizeof(__u32));
    __uint(value_size, sizeof(__u32));
} events SEC(".maps");

SEC("tracepoint/syscalls/sys_enter_openat")
int trace_openat(struct sys_enter_openat_ctx *ctx)
{
    struct event e = {};

    /* Capture PID (upper 32 bits of the combined pid_tgid value). */
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    e.pid = (__u32)(pid_tgid >> 32);

    /* Capture the process name from the kernel's task_struct. */
    bpf_get_current_comm(e.comm, sizeof(e.comm));

    /* args[1] holds the user-space pointer to the filename string. */
    const char *user_filename = (const char *)ctx->args[1];
    bpf_probe_read_user_str(e.filename, sizeof(e.filename), user_filename);

    /* args[2] holds the openat flags (O_RDONLY=0, O_WRONLY=1, O_RDWR=2). */
    e.flags = (__u32)ctx->args[2];

    /*
     * Kernel-side filter: drop events whose filename does not begin with
     * "/proc/sys" to avoid emitting noise for unrelated file accesses.
     */
    if (e.filename[0] != '/' ||
        e.filename[1] != 'p' ||
        e.filename[2] != 'r' ||
        e.filename[3] != 'o' ||
        e.filename[4] != 'c' ||
        e.filename[5] != '/' ||
        e.filename[6] != 's' ||
        e.filename[7] != 'y' ||
        e.filename[8] != 's')
    {
        return 0;
    }

    /* Emit the event on the current CPU's perf ring buffer slot. */
    bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, &e, sizeof(e));

    return 0;
}

char LICENSE[] SEC("license") = "GPL";
