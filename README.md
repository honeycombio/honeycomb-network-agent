# Honeycomb eBPF Agent for Kubernetes

<!-- OSS metadata badge - rename repo link and set status in OSSMETADATA -->
<!-- [![OSS Lifecycle](https://img.shields.io/osslifecycle/honeycombio/{repo-name})](https://github.com/honeycombio/home/blob/main/honeycomb-oss-lifecycle-and-practices.md) -->

The agent is deployed to Kubernetes as a [`DaemonSet`](https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/),
which means that Kubernetes will try to have the agent run on every node in the cluster.

Docker images are found in [`ghcr.io/honeycombio/ebpf-agent:latest`](https://github.com/honeycombio/honeycomb-ebpf-agent/pkgs/container/ebpf-agent).

See notes on local development in [`DEVELOPING.md`](./DEVELOPING.md)
