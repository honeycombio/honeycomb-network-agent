# Developing

## Prerequisites

Required:

- Docker and Go

Recommended:

- [Docker Desktop](https://www.docker.com/products/docker-desktop/)
- [K9s](https://k9scli.io/) - A Terminal UI for Kubernetes
- [remake](https://remake.readthedocs.io/) - A better make
  - View them with `remake --tasks`
- [locust](https://docs.locust.io/en/stable/what-is-locust.html) - A performance testing tool, required for load testing

## Local Development

When making changes to C files, run `make docker-generate` to update the generated go files.
For example, run it after changing the `tcp_event` struct in `tcp_probe.c`.

When building with `make docker-build`, the generated files are included in the build but not updated locally.

## To pull a published image from ghcr

Docker images are found in [`ghcr.io/honeycombio/network-agent:latest`](https://github.com/honeycombio/honeycomb-network-agent/pkgs/container/network-agent).

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

`make docker-build` will create a local docker image called `hny/network-agent:local`.

Verify that it published to your local docker images:

```sh
$ docker images | grep network-agent
REPOSITORY           TAG       IMAGE ID        CREATED          SIZE
hny/network-agent    local     326362e52d9c    5 minutes ago    120MB
```

For a custom name and/or tag, pass `IMG_NAME` and/or `IMG_TAG` in the make command.
For example, to get a local docker image called `hny/network-agent-go:custom`:

`IMG_NAME=hny/network-agent-go IMG_TAG=custom make docker-build`

## Deploying the agent to a Kubernetes cluster

Set environment variables like `HONEYCOMB_API_KEY` and the previously noted `GITHUB_TOKEN` and `BASE64_TOKEN` in a file called `.env`.
These environment variables get passed in the make command.

`make apply-network-agent`

```sh
$ make apply-network-agent
namespace/honeycomb created
secret/honeycomb created
secret/ghcr created
daemonset.apps/hny-network-agent created
```

If you're on a Mac, try `brew install gettext` if `envsubst` isn't available.

Confirm that the pods are up by using `k9s` or with `kubectl`:

```sh
$ kubectl get pods --namespace=honeycomb
NAME                      READY   STATUS    RESTARTS   AGE
hny-network-agent-bqcvl   1/1     Running   0          94s
```

To remove the agent:

`make unapply-network-agent` or `kubectl delete -f smoke-tests/deployment.yaml`

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

## Load Testing

After a locally built image and an API key is set:

`make swarm`

This will apply the agent, apply the echoserver, and start locust.

Navigate to `http://0.0.0.0:8089/` in your browser and set users and spawn rate, e.g. 5000 and 100, and hit Start swarming.

To tear down the load test, `ctrl+c` in the terminal running and `make unswarm`.

See more details in [`smoke-tests/loadtest.md`](./smoke-tests/loadtest.md)

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

## Updating bpf header files

Update the version in `bpf/headers/update.sh`.

`make update-headers` or run `cd bpf/headers && ./update.sh`

Fix line in `bpf_tracing.h` from `#include <bpf/bpf_helpers.h>` to `#include "bpf_helpers.h"`

## Generating vmlinux.h files

`vmlinux.h` files contain all the linux types and structs to interop with a linux OS, e.g. the raw Socket class.

We need a version for each supported architecture (eg arm & amd) and it's generated from a real linux distro.

Steps to generate `vmlinux.h` files:

- Start a ubuntu VM (not docker, use virtualbox, multipass, ec2, etc)
- Install additional linux commands so libbpf can work - `apt install linux-tools-$(uname -r)`
- Use libbpf to generate the vmlinux.h file - `bpftool btf dump file /sys/kernel/btf/vmlinux format c`
- Check in output vmlinux.h, note which architecture in file format - eg `bpf/headers/vmlinux-arm64.h`
