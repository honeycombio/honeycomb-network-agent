# Developing

From pod terminal, which must be run in privileged mode:

```sh
# Print kernel messages:
dmesg
# Mount debugfs:
mount -t debugfs nodev /sys/kernel/debug
# Check:
mount | grep -i debugfs
# output:
cat /sys/kernel/debug/tracing/trace_pipe
# look around sys/kernel/debug
```
