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

Build with `make docker-build`.

## To pull a published image from ghcr

Docker images are found in [`ghcr.io/honeycombio/network-agent:latest`](https://github.com/honeycombio/honeycomb-network-agent/pkgs/container/network-agent).

```sh
docker pull ghcr.io/honeycombio/network-agent:latest
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

Set environment variables like `HONEYCOMB_API_KEY` in a file called `.env`.
These environment variables get passed in the make command.

`make apply-network-agent`

```sh
$ make apply-network-agent
namespace/honeycomb created
secret/honeycomb created
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

Set `LOG_LEVEL=DEBUG` to generate debug log statements in the agent logs.

### Debug Service

The agent includes an optional debug service that can be used with any tools that can collect pprof data.

The debug service is generally only used when debugging the agent itself, and will only run if the `DEBUG` environment variable is set to `true`.

`DEBUG_ADDRESS` is the IP and port where the debug service runs.
If this value is not specified, then the debug service runs on the first open port between `0.0.0.0:6060` and `0.0.0.0:6069`.

```sh
DEBUG=true
DEBUG_ADDRESS="1.2.3.4:1234"
```

## Gopacket

We maintain a fork of [gopacket/gopacket](https://github.com/gopacket/gopacket) as [honeycombio/gopacket](https://github.com/honeycombio/gopacket).
The agent is configured to use the official gopacket repo as part of its main dependency chain and import paths.
The Honeycomb fork is swapped in using a `replace` directive in `go.mod`.
This allows the fork to remain cleaner, easier to manage and makes it easier to provide upstream contributions.
We will not be doing releases on our fork of gopacket, and instead here will use specific commit shas from our fork.

### Updating gopacket

- Go to our fork and identify the commit sha you want to update to, which will be used in the next step.
- Run `go get github.com/honeycombio/gopacket@<commit-sha>`

The above command will fail because of a module name mismatch, but it will print the full pseudo version/commit SHA that Go found as a result of that command.

For example:

```shell
$ go get github.com/honeycombio/gopacket@82dde036188549768ff5b13414ff8a7441b9a17f
go: github.com/honeycombio/gopacket@v1.1.2-0.20230914230614-82dde0361885: parsing go.mod:
  module declares its path as: github.com/gopacket/gopacket
    but was required as: github.com/honeycombio/gopacket
```

`v1.1.2-0.20230914230614-82dde0361885` is the "version" we want to replace upstream with in the next step.

- Edit `go.mod` to update the `replace` directive for gopacket's pseudo version.

For example

```golang
replace github.com/gopacket/gopacket => github.com/honeycombio/gopacket v1.1.2-0.20230914230614-82dde0361885
```

- Run `go mod tidy`
