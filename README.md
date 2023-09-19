# Honeycomb Network Agent for Kubernetes

[![OSS Lifecycle](https://img.shields.io/osslifecycle/honeycombio/honeycomb-network-agent)](https://github.com/honeycombio/home/blob/main/honeycomb-oss-lifecycle-and-practices.md)

The agent is deployed to Kubernetes as a [`DaemonSet`](https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/),
which means that Kubernetes will try to have the agent run on every node in the cluster.

Docker images are found in [`ghcr.io/honeycombio/network-agent:latest`](https://github.com/honeycombio/honeycomb-network-agent/pkgs/container/network-agent).

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


### Configuration

The network agent can be configured using the following environment variables.

| Environment Variable | Description | Default | Required |
| -------------------- | ----------- | ------- | -------- |
| `HONEYCOMB_API_KEY` | The Honeycomb API key used when sending events | `` (empty) | `true` |
| `HONEYCOMB_API_ENDPOINT` | The endpoint to send events to | `https://api.honeycomb.io` | `false` |
| `HONEYCOMB_DATASET` | Dataset where network events are stored | `hny-network-agent` | `false` |
| `HONEYCOMB_STATS_DATASET` | Dataset where operational statistics for the network agent are stored  | `hny-network-agent-stats` | `false` |
| `LOG_LEVEL`  | The log level to use when printing logs to console | `INFO` | `false` |
| `DEBUG` | Runs the agent in debug mode including enabling a profiling endpoint using Debug Address  | `false` | `false` |
| `DEBUG_ADDRESS` | The endpoint to listen to when running the profile endpoint | `localhost:6060` | `false` |

### Run

```sh
kubectl apply -f examples/quickstart.yaml
```

Events should show up in Honeycomb in the `hny-network-agent` dataset.

Alternative options for configuration and running can be found in [Deploying the agent to a Kubernetes cluster](./DEVELOPING.md#deploying-the-agent-to-a-kubernetes-cluster):

## Example Event

```json
{
  "Timestamp": "2023-08-24T19:42:17.65267Z",
  "destination.address": "192.168.65.4",
  "destination.k8s.pod.name": "storage-provisioner",
  "destination.k8s.pod.uid": "87e44763-257b-4a52-91cc-f959292ff416",
  "duration_ms": 11,
  "goroutine_count": 274,
  "honeycomb.agent_version": "0.0.6-alpha",
  "http.method": "GET",
  "http.request.body.size": 0,
  "http.response.body.size": 25,
  "http.status_code": 200,
  "http.url": "/greeting",
  "httpEvent_handled_at": "2023-08-24T19:42:18.575463885Z",
  "httpEvent_handled_latency_ms": 922,
  "k8s.container.name": "frontend",
  "k8s.namespace.name": "greetings",
  "k8s.node.name": "docker-desktop",
  "k8s.node.uid": "54fe4a74-f29d-49ba-815f-a411bab8e166",
  "k8s.pod.name": "frontend-go-646c6f4b7d-848xz",
  "k8s.pod.uid": "faed166d-b598-48fc-b1f9-45f4bbaa92b7",
  "k8s.service.name": "frontend",
  "meta.btf_enabled": false,
  "meta.kernel_version": "5.15.49",
  "name": "HTTP GET",
  "net.sock.host.addr": "10.1.3.192",
  "user_agent.original": "curl/8.1.2"
}
```

## Supported Versions

- Kubernetes version 1.24+
- Linux Kernel 5.10+ with BPF, PERFMON, and NET_RAW capabilities

Other versions may work but these are the minimum versions currently being tested.
