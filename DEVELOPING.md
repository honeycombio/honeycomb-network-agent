### Generating vmlinux.h files

vmlinux.h files contain all the linux types and structs to interop with a linux OS, eg the raw Socket class.

We need a version for each supported architecture (eg arm & amd) and it's generated from a real linux distro.

Steps to generate vmlinx.h

- Start a ubuntu VM (not docker, use virtualbox, multipass, ec2, etc)
- Install additional linux commands to libbpf can work - `apt install linux-tools-$(uname -r)`
- Use libbpf to generate the vmlinx.h file - `bpftool btf dump file /sys/kernel/btf/vmlinux format c`
- Check in output vmlinux.h, note which architecure in file format - eg `bpf/headers/vmlinux-arm64.h`
