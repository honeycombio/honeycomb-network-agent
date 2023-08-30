# Honeycomb eBPF Agent changelog

## [0.0.7-alpha] - 2023-08-30

### Maintenance

- maint: add debug telemetry (#136) | @pkanal
- chore: log when we've got an event where time's gone wobbly (#137) | @robbkidd
- docs: add Quickstart docs and example (#89) | @JamieDanielson

## [0.0.6-alpha] - 2023-08-24

### Enhancements

- Clean up pcap handles and allow alternative sources #103 | @MikeGoldsmith

### Fixes

- Improve request / response handling (#117) | @MikeGoldsmith
- Remove map entry after finding match (#124) | @MikeGoldsmith
- Remove fields for http body and headers (#113) | @JamieDanielson

### Maintenance

- Fix typo in request counter (#123) | @MikeGoldsmith

## [0.0.5-alpha] - 2023-08-22

### Fixes

fix: prevent debug log panic (#108) | @pkanal

### Maintenance

maint: Move stream flushing to ticker (#104) | @MikeGoldsmith
maint: Don't deploy kprobes for now (#106) | @MikeGoldsmith
maint: Bump k8s.io libraries to 0.28.0 (#105) | @MikeGoldsmith

## [0.0.4-alpha] - 2023-08-21

### Enhancements

- Send request/response body size (#80) | @pkanal
- More better logging (#72) | @robbkidd
- Send user agent as a separate attribute (#79) | @pkanal
- Use timestamps packet was captured (#75)  | @MikeGoldsmith
- Print config as indented JSON on startup (#73)  | @MikeGoldsmith
- Add k8s metadata to gopacket events (#71) | @pkanal
- Request / Response matching (#70) | @pkanal
- Break up TCP assembly into components and move to assemblers directory (#65) | @MikeGoldsmith
- Add TCP stream reader using gopacket (#62) | @MikeGoldsmith
- Add cached k8s client (#84) | @MikeGoldsmith

### Fixes

- Update TCP connection timeout to 30 seconds (#94) | @MikeGoldsmith
- Time units in telemetry (#95) | @vreynolds
- Separate event transform function (#81) | @robbkidd

### Maintenance

- Don’t set priviledged mode by default in deployment.yaml (#82) | @MikeGoldsmith
- Clean up probes manager (#83) | @MikeGoldsmith
- Update create Github Release job to have write contents access (#58) | @MikeGoldsmith
- Detect BTF support and disable for local dev (#61) | @MikeGoldsmith
- Readd contents read permission to release workflow (#101) | @MikeGoldsmith

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

## [0.0.2-alpha] - 2023-07-27

### Enhancements

- feat: map IP address to pod name (#42) | @pkanal

### Maintenance

- maint: add developing notes and comments (#41) | @JamieDanielson

## [0.0.1-alpha] - 2023-07-18

Initial release
