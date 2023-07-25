# Developing

## Prerequisites

Required:

- Docker and Go

Recommended:

- [Docker Desktop](https://www.docker.com/products/docker-desktop/)
- [K9s](https://k9scli.io/) - A Terminal UI for Kubernetes
- [remake](https://remake.readthedocs.io/) - A better make
  - View them with `remake --tasks`

## To pull a published image from ghcr

Docker images are found in [`ghcr.io/honeycombio/ebpf-agent:latest`](https://github.com/honeycombio/honeycomb-ebpf-agent/pkgs/container/ebpf-agent).

Because this is a private registry, you must have a Github [personal access token](https://github.com/settings/tokens) (classic) with `read:packages` permission.

The example deployment creates the secret using environment variables (similar to how it mounts the API Key secret below).

```sh
export GITHUB_TOKEN=githubusername:githubaccesstoken
export BASE64_TOKEN=$(echo -n $GITHUB_TOKEN | base64)
```

An alternative way to create the secret is to comment out the sections for `ghcr-secret` in the deployment and create it manually:

```sh
kubectl create secret docker-registry ghcr-secret \
  --docker-server=https://ghcr.io/ \
  --docker-username=<githubusername> \
  --docker-password=<githubaccesstoken> \
  --namespace=honeycomb
```

## To create a local docker image

`make docker-build` will create a docker image called `hny/ebpf-agent:local``.

For a custom name and/or tag, pass `IMG_NAME` and/or `IMG_TAG` in the make command.

For example, to get a local docker image called `hny/ebpf-agent-go:custom`:

`IMG_NAME=hny/ebpf-agent-go IMG_TAG=custom make docker-build`

## Deploying the agent to a Kubernetes cluster

Set environment variables like `HONEYCOMB_API_KEY` in a file called `.env`.
These environment variables get passed in the make command.

`make apply-ebpf-agent`

## Remove the agent

`make unapply-ebpf-agent`

## Debugging

From an agent pod terminal, which must be run in privileged mode:

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
