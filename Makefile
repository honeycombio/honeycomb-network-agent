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
generate:
	go generate ./...

.PHONY: docker-generate
docker-generate:
	docker build --tag hny/ebpf-agent-builder . -f bpf/Dockerfile
	docker run --rm -v $(shell pwd):/src hny/ebpf-agent-builder

.PHONY: build
build:
	CGO_ENABLED=1 GOOS=linux go build -o hny-ebpf-agent main.go

.PHONY: docker-build
docker-build:
	docker build --tag $(IMG_NAME):$(IMG_TAG) .

.PHONY: update-headers
update-headers:
	cd bpf/headers && ./update.sh
	@echo "*** Also update bpf_tracing.h file! ***"

### Testing targets

# deploy ebpf agent daemonset to already-running cluster with env vars from .env file
.PHONY: apply-ebpf-agent
apply-ebpf-agent:
	envsubst < smoke-tests/deployment.yaml | kubectl apply -f -

# remove ebpf agent daemonset
.PHONY: unapply-ebpf-agent
unapply-ebpf-agent:
	kubectl delete -f smoke-tests/deployment.yaml

# apply new greetings deployment in already-running cluster
.PHONY: apply-greetings
apply-greetings:
	kubectl apply -f smoke-tests/greetings.yaml

# remove greetings deployment
.PHONY: unapply-greetings
unapply-greetings:
	kubectl delete -f smoke-tests/greetings.yaml

# deploy echoserver in already-running cluster and start locust
.PHONY: loadtest
loadtest: apply-ebpf-agent
	kubectl apply -f smoke-tests/echoserver.yaml
	cd smoke-tests && locust

# teardown load test setup
.PHONY: unload
unload:
	kubectl delete -f smoke-tests/echoserver.yaml
	make unapply-ebpf-agent
