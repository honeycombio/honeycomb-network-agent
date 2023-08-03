# Honeycomb eBPF Agent changelog

## [0.0.3-alpha] - 2023-08-03

### Enhancements

- feat: Add more Kubernetes metadata (#46) | @pkanal
- feat: Log and add agent, kernel and btfEnabled to events (#52) | @JamieDanielson
- feat: Add vmlinux for kernel 5.15 for arm & amd (#26) | @MikeGoldsmith

### Fixes

- fix: fix source IP address (#43) | @JamieDanielson

### Maintenance

- maint: rename socket_event to tcp_event (#51) | @JamieDanielson
- maint: update bpf header files to latest (#50) | @JamieDanielson
- maint(deps): bump github.com/cilium/ebpf from 0.10.0 to 0.11.0 (#21)

## [0.0.2] - 2023-07-27

### Enhancements

- feat: map IP address to pod name (#42) | @pkanal

### Maintenance

- maint: add developing notes and comments (#41) | @JamieDanielson

## [0.0.1] - 2023-07-18

Initial release
