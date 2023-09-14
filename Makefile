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

IMG_NAME ?= hny/ebpf-agent
IMG_TAG ?= local

.PHONY: generate
generate: export CFLAGS := $(BPF_HEADERS)
#: generate go/bpf interop code
generate:
	go generate ./...

.PHONY: docker-generate
#: generate go/bpf interop code but in Docker
docker-generate:
	docker build --tag hny/ebpf-agent-builder . -f bpf/Dockerfile
	docker run --rm -v $(shell pwd):/src hny/ebpf-agent-builder

.PHONY: build
#: compile the agent executable
build:
	CGO_ENABLED=1 GOOS=linux go build -o hny-ebpf-agent main.go

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
#: deploy ebpf agent daemonset to already-running cluster with env vars from .env file
apply-agent:
	envsubst < smoke-tests/deployment.yaml | kubectl apply -f -

.PHONY: unapply-agent
#: remove ebpf agent daemonset
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
