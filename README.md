# Honeycomb eBPF Agent for Kubernetes

<!-- OSS metadata badge - rename repo link and set status in OSSMETADATA -->
<!-- [![OSS Lifecycle](https://img.shields.io/osslifecycle/honeycombio/{repo-name})](https://github.com/honeycombio/home/blob/main/honeycomb-oss-lifecycle-and-practices.md) -->

The agent is deployed to Kubernetes as a [`DaemonSet`](https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/),
which means that Kubernetes will try to have the agent run on every node in the cluster.

Docker images are found in [`ghcr.io/honeycombio/ebpf-agent:latest`](https://github.com/honeycombio/honeycomb-ebpf-agent/pkgs/container/ebpf-agent).

See notes on local development in [`DEVELOPING.md`](./DEVELOPING.md)

## Getting Started (Quickstart)

### Requirements

- A running Kubernetes cluster (see [Supported Versions](#supported-versions))
- A Honeycomb API Key
- A classic [personal access token](https://github.com/settings/tokens) from GitHub with `read:packages` permission

### Setup

Create honeycomb namespace:

```sh
kubectl apply -f examples/ns.yaml
```

Create secret for `HONEYCOMB_API_KEY` (alternatively, use `examples/secret-honeycomb.yaml`):

```sh
export HONEYCOMB_API_KEY=mykey
kubectl create secret generic honeycomb --from-literal=api-key=$HONEYCOMB_API_KEY --namespace=honeycomb
```

Create secret for ghcr login (alternatively, use `examples/secret-ghcr.yaml`):

```sh
export GITHUB_USERNAME=githubusername
export GITHUB_ACCESS_TOKEN=githubaccesstoken
kubectl create secret docker-registry ghcr-secret \
  --docker-server=https://ghcr.io/ \
  --docker-username=$GITHUB_USERNAME \
  --docker-password=$GITHUB_ACCESS_TOKEN \
  --namespace=honeycomb
```

### Run

```sh
kubectl apply -f examples/quickstart.yaml
```

Events should show up in Honeycomb in the `hny-ebpf-agent` dataset.

Alternative options for configuration and running can be found in [Deploying the agent to a Kubernetes cluster](./DEVELOPING.md#deploying-the-agent-to-a-kubernetes-cluster):

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
