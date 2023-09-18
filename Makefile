# Obtain an absolute path to the directory of the Makefile.
# Assume the Makefile is in the root of the repository.
REPODIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

# Build the list of header directories to compile the bpf program
BPF_HEADERS += -I${REPODIR}/bpf/headers

# Disable BTF if the kernel doesn't support it (eg local dev on Docker Desktop)
# needed until BTF is enabled for Docker Desktop
# see https://github.com/docker/for-mac/issues/6800
ifeq (,$(wildcard /sys/kernel/btf/vmlinux))
	BPF_HEADERS += -DBPF_NO_PRESERVE_ACCESS_INDEX
endif

# I_MADE_THIS_UP ?= $(shell git describe --always --match "v[0-9]*")
I_MADE_THIS_UP ?= v0.0.14-alpha-8-g57c7eb0
IMG_NAME ?= hny/network-agent
IMG_TAG ?= local

.PHONY: version
#: display the current computed project version
version:
	@echo $(I_MADE_THIS_UP)

.PHONY: generate
generate: export CFLAGS := $(BPF_HEADERS)
#: generate go/bpf interop code
generate:
	go generate ./...

.PHONY: docker-generate
#: generate go/bpf interop code but in Docker
docker-generate:
	docker build --tag hny/network-agent-builder . -f bpf/Dockerfile
	docker run --rm -v $(shell pwd):/src hny/network-agent-builder

.PHONY: build
#: compile the agent executable
build:
	CGO_ENABLED=1 GOOS=linux \
		go build \
			--ldflags "-X main.Version=v0.0.14-alpha-8-g57c7eb0" \
			-o hny-network-agent main.go

.PHONY: docker-build
#: build the agent image
docker-build:
	docker build --tag $(IMG_NAME):$(IMG_TAG) .

.PHONY: update-headers
#: retrieve libbpf headers
update-headers:
	cd bpf/headers && ./update.sh
	@echo "*** Also update bpf_tracing.h file! ***"

### Testing targets

.PHONY: apply-agent
#: deploy network agent daemonset to already-running cluster with env vars from .env file
apply-agent:
	envsubst < smoke-tests/deployment.yaml | kubectl apply -f -

.PHONY: unapply-agent
#: remove network agent daemonset
unapply-agent:
	kubectl delete -f smoke-tests/deployment.yaml

.PHONY: apply-greetings
#: apply new greetings deployment in already-running cluster
apply-greetings:
	kubectl apply -f smoke-tests/greetings.yaml

.PHONY: unapply-greetings
#: remove greetings deployment
unapply-greetings:
	kubectl delete -f smoke-tests/greetings.yaml

.PHONY: apply-echoserver
#: deploy echoserver in already-running cluster
apply-echoserver:
	kubectl apply -f smoke-tests/echoserver.yaml

.PHONY: unapply-echoserver
#: remove echoserver
unapply-echoserver:
	kubectl delete -f smoke-tests/echoserver.yaml

.PHONY: swarm
#: run agent and echoserver, then run load test
swarm: apply-agent apply-echoserver
	cd smoke-tests && locust

.PHONY: unswarm
#: teardown load test agent and echoserver
unswarm: unapply-echoserver unapply-agent

.PHONY: apply-pyroscope-server
#: spin up a pyroscope server in k8s cluster
apply-pyroscope-server:
	helm repo add pyroscope-io https://pyroscope-io.github.io/helm-chart
	helm repo update
	helm install pyroscope pyroscope-io/pyroscope -f smoke-tests/pyroscope_values.yaml

.PHONY: port-forward-pyroscope
#: port forward pyroscope server to localhost:4040. doesnt work, run manually.
port-forward-pyroscope:
	export POD_NAME=$(kubectl get pods --namespace default -l "app.kubernetes.io/name=pyroscope,app.kubernetes.io/instance=pyroscope" -o jsonpath="{.items[0].metadata.name}")
	export CONTAINER_PORT=$(kubectl get pod --namespace default $POD_NAME -o jsonpath="{.spec.containers[0].ports[0].containerPort}")
	kubectl --namespace default port-forward $POD_NAME 4040:$CONTAINER_PORT

.PHONY: unapply-pyroscope-server
#: tear down pyroscope server
unapply-pyroscope-server:
	helm uninstall pyroscope
