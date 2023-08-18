# Honeycomb eBPF Agent for Kubernetes

<!-- OSS metadata badge - rename repo link and set status in OSSMETADATA -->
<!-- [![OSS Lifecycle](https://img.shields.io/osslifecycle/honeycombio/{repo-name})](https://github.com/honeycombio/home/blob/main/honeycomb-oss-lifecycle-and-practices.md) -->

The agent is deployed to Kubernetes as a [`DaemonSet`](https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/),
which means that Kubernetes will try to have the agent run on every node in the cluster.

Docker images are found in [`ghcr.io/honeycombio/ebpf-agent:latest`](https://github.com/honeycombio/honeycomb-ebpf-agent/pkgs/container/ebpf-agent).

See notes on local development in [`DEVELOPING.md`](./DEVELOPING.md)

## Getting Started

### Requirements

- A running Kubernetes cluster (see [Supported Versions](#supported-versions))
- A Honeycomb API Key
- A classic [personal access token](https://github.com/settings/tokens) from GitHub with `read:packages` permission

### Setup

- Copy `.env.example` to new file `.env`
- In `.env`, set your `HONEYCOMB_API_KEY` with your API Key from Honeycomb
- In `.env`, set your `GITHUB_TOKEN` (See [To pull a published image from ghcr](./DEVELOPING.md#to-pull-a-published-image-from-ghcr) for more detail)
- In `deployment.yaml`, set your preferred `HONEYCOMB_DATASET`
- In `deployment.yaml`, set the image to the version you want to use, e.g. `ghcr.io/honeycombio/ebpf-agent:v0.0.3-alpha`

### Run

- Apply with one of these methods (See [Deploying the agent to a Kubernetes cluster](./DEVELOPING.md#deploying-the-agent-to-a-kubernetes-cluster) for more detail):
  - `make apply-ebpf-agent`
  - `envsubst < deployment.yaml | kubectl apply -f -`
- Check out your data in Honeycomb!

## Example Event

```json
{
  "destination.address": "192.168.65.4",
  "duration_ms": 4937,
  "goroutine_count": 540,
  "honeycomb.agent_version": "0.0.3-alpha",
  "http.method": "GET",
  "http.request.body": "{}",
  "http.request.body.size": 0,
  "http.request.headers": "map[Accept:[*/*] User-Agent:[curl/8.1.2]]",
  "http.response.body": "{}",
  "http.response.body.size": 24,
  "http.response.headers": "{\"Content-Length\":[\"24\"],\"Content-Type\":[\"text/plain; charset=utf-8\"],\"Date\":[\"Fri, 18 Aug 2023 18:26:01 GMT\"]}",
  "http.status_code": 200,
  "http.url": "/greeting",
  "httpEvent_handled_at": "2023-08-18T18:26:01.438160846Z",
  "httpEvent_handled_latency": 11974667,
  "meta.btf_enabled": false,
  "meta.kernel_version": "5.15.49",
  "name": "HTTP GET",
  "net.sock.host.addr": "10.1.3.82",
  "user_agent.original": "curl/8.1.2"
}
```

## Supported Versions

- Kubernetes version 1.24+
- Linux Kernel 5.10+ with BPF, PERFMON, and NET_RAW capabilities

Other versions may work but these are the minimum versions currently being tested.
