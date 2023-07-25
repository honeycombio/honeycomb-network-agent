# Obtain an absolute path to the directory of the Makefile.
# Assume the Makefile is in the root of the repository.
REPODIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

# Build the list of header directories to compile the bpf program
BPF_HEADERS += -I${REPODIR}/bpf/headers

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
build: generate
	CGO_ENABLED=0 GOOS=linux go build -o hny-ebpf-agent main.go

.PHONY: docker-build
docker-build:
	docker build --tag $(IMG_NAME):$(IMG_TAG) .

### Testing targets

# deploy ebpf agent daemonset to already-running cluster with env vars from .env file
.PHONY: apply-ebpf-agent
apply-ebpf-agent:
	envsubst < deployment.yaml | kubectl apply -f -

# remove ebpf agent daemonset
.PHONY: unapply-ebpf-agent
unapply-ebpf-agent:
	kubectl delete -f deployment.yaml

# apply new greetings deployment in already-running cluster
.PHONY: apply_greetings
apply_greetings:
	kubectl apply -f smoke-tests/greetings.yaml

# remove greetings deployment
.PHONY: unapply_greetings
unapply_greetings:
	kubectl delete -f smoke-tests/greetings.yaml
