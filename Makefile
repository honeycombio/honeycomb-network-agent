REPODIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
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

.PHONY: apply-ebpf-agent
apply-ebpf-agent:
	# apply new deployment in already-running cluster
	# load locally built image into kind
	# kind load docker-image hny/ebpf-agent:local
	# replace env vars in ebpf_agent.yaml (eg API key) and apply deployment
	envsubst < deployment.yaml | kubectl apply -f -

.PHONY: unapply-ebpf-agent
unapply-ebpf-agent:
	kubectl delete -f deployment.yaml
