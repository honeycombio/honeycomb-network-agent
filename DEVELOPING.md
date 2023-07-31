# Developing

## Prerequisites

Required:

- Docker and Go

Recommended:

- [Docker Desktop](https://www.docker.com/products/docker-desktop/)
- [K9s](https://k9scli.io/) - A Terminal UI for Kubernetes
- [remake](https://remake.readthedocs.io/) - A better make
  - View them with `remake --tasks`

## Local Development

When making changes to C files, run `make docker-generate` to update the generated go files.
For example, run it after changing the `socket_event` struct in `tcp_probe.c`.

When building with `make docker-build`, the generated files are included in the build but not updated locally.

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

`make docker-build` will create a local docker image called `hny/ebpf-agent:local`.

Verify that it published to your local docker images:

```sh
$ docker images | grep ebpf-agent
REPOSITORY        TAG       IMAGE ID        CREATED          SIZE
hny/ebpf-agent    local     326362e52d9c    5 minutes ago    120MB
```

For a custom name and/or tag, pass `IMG_NAME` and/or `IMG_TAG` in the make command.
For example, to get a local docker image called `hny/ebpf-agent-go:custom`:

`IMG_NAME=hny/ebpf-agent-go IMG_TAG=custom make docker-build`

## Deploying the agent to a Kubernetes cluster

Set environment variables like `HONEYCOMB_API_KEY` and the previously noted `GITHUB_TOKEN` and `BASE64_TOKEN` in a file called `.env`.
These environment variables get passed in the make command.

`make apply-ebpf-agent`

```sh
$ make apply-ebpf-agent
namespace/honeycomb created
secret/honeycomb-secrets created
secret/ghcr created
daemonset.apps/hny-ebpf-agent created
```

Confirm that the pods are up by using `k9s` or with `kubectl`:

```sh
$ kubectl get pods --namespace=honeycomb
NAME                   READY   STATUS    RESTARTS   AGE
hny-ebpf-agent-bqcvl   1/1     Running   0          94s
```

To remove the agent:

`make unapply-ebpf-agent` or `kubectl delete -f deployment.yaml`

## Optionally install the "greetings" example app

There is an example greeting service written in go that can be used to see additional telemetry.

`make apply-greetings` or `kubectl apply -f smoke-tests/greetings.yaml`

Confirm the pods are up by using `k9s` or with `kubectl`:

```sh
$ kubectl get pods --namespace=greetings
NAME                           READY   STATUS    RESTARTS   AGE
frontend-go-6cb864498b-wrpzj   1/1     Running   0          12s
message-go-b7fcd59d4-jwrhc     1/1     Running   0          12s
name-go-5794ffd766-4qxp2       1/1     Running   0          12s
year-go-b96849dc6-xjfts        1/1     Running   0          12s
```

Hit the endpoint:

`curl localhost:7777/greeting`

## Remove the "greetings" example app

`make unapply-greetings` or `kubectl delete -f smoke-tests/greetings.yaml`

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
