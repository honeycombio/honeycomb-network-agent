# Honeycomb Network Agent for Kubernetes

[![OSS Lifecycle](https://img.shields.io/osslifecycle/honeycombio/honeycomb-network-agent)](https://github.com/honeycombio/home/blob/main/honeycomb-oss-lifecycle-and-practices.md)

The agent is deployed to Kubernetes as a [`DaemonSet`](https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/),
which means that Kubernetes will try to have the agent run on every node in the cluster.

Docker images are found in [`ghcr.io/honeycombio/network-agent:latest`](https://github.com/honeycombio/honeycomb-network-agent/pkgs/container/network-agent).

See notes on local development in [`DEVELOPING.md`](./DEVELOPING.md)

## Getting Started (Quickstart)

### Requirements

- A running Kubernetes cluster (see [Supported Platforms](#supported-platforms))
- A Honeycomb API Key

### Setup

Create Honeycomb namespace for the agent to run in:

```sh
kubectl create namespace honeycomb
```

Create Honeycomb secret for `HONEYCOMB_API_KEY` environment variable so it can be passed into the agent:

```sh
export HONEYCOMB_API_KEY=mykey
kubectl create secret generic honeycomb --from-literal=api-key=$HONEYCOMB_API_KEY --namespace=honeycomb
```

### Configuration

The network agent can be configured using the following environment variables.

| Environment Variable      | Description                                                                              | Default                    | Required? |
| ------------------------- | ---------------------------------------------------------------------------------------- | -------------------------- | --------- |
| `HONEYCOMB_API_KEY`       | The Honeycomb API key used when sending events                                           | `` (empty)                 | **Yes**   |
| `HONEYCOMB_API_ENDPOINT`  | The endpoint to send events to                                                           | `https://api.honeycomb.io` | No        |
| `HONEYCOMB_DATASET`       | Dataset where network events are stored                                                  | `hny-network-agent`        | No        |
| `HONEYCOMB_STATS_DATASET` | Dataset where operational statistics for the network agent are stored                    | `hny-network-agent-stats`  | No        |
| `LOG_LEVEL`               | The log level to use when printing logs to console                                       | `INFO`                     | No        |
| `DEBUG`                   | Runs the agent in debug mode including enabling a profiling endpoint using Debug Address | `false`                    | No        |
| `DEBUG_ADDRESS`           | The endpoint to listen to when running the profile endpoint                              | `localhost:6060`           | No        |

### Run

```sh
kubectl apply -f examples/quickstart.yaml
```

Events should show up in Honeycomb in the `hny-network-agent` dataset.

Alternative options for configuration and running can be found in [Deploying the agent to a Kubernetes cluster](./DEVELOPING.md#deploying-the-agent-to-a-kubernetes-cluster):

## Supported Platforms

| Platform                                                             | Supported                             |
| ---------------------------------------------------------------------| ------------------------------------- |
| [AKS](https://azure.microsoft.com/en-gb/products/kubernetes-service) | Supported ✅                          | 
| [EKS](https://aws.amazon.com/eks/)                                   | Self-managed hosts ✅ <br> Fargate ❌  |
| [GKE](https://cloud.google.com/kubernetes-engine)                    | Standard cluster ✅ <br> AutoPilot ❌  |
| Self-hosted                                                          | Ubuntu ✅                             |

### Requirements

- Kubernetes version 1.24+
- Linux Kernel 5.10+ with NET_RAW capabilities

Other versions may work but these are the minimum versions currently being tested.
