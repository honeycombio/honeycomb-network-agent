IMG_NAME ?= hny/network-agent
IMG_TAG ?= local

.PHONY: build
#: compile the agent executable
build:
	CGO_ENABLED=1 GOOS=linux go build -o hny-network-agent main.go

.PHONY: docker-build
#: build the agent image
docker-build:
	docker build --tag $(IMG_NAME):$(IMG_TAG) .

.PHONY: test
#: run unit tests
test:
	go test ./... -count=1

.PHONY: docker-test
#: run unit tests in docker
docker-test:
	docker build --target test .

### Testing targets

.PHONY: smoke
#: run smoke tests - for local tests comment out docker-build to save time, for CI uncomment docker-build
smoke:  smokey_cluster_create smokey_collector_install smokey_agent_install smokey_echo_job smokey_copy_output smokey_verify_output

smokey_cluster_create:
	kind create cluster

smokey_collector_install:
  # install opentelemetry collector using helm chart and wait for it to be ready
	helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts
	helm install smokey-collector open-telemetry/opentelemetry-collector --values smoke-tests/collector-helm-values.yaml
	kubectl rollout status statefulset.apps/smokey-collector-opentelemetry-collector --timeout=60s

# If the image doesn't exist, return the name of the make target to build it.
# Use this as a prerequisite to any target that needs the image to exist.
maybe_docker_build = $(if $(shell docker images -q $(IMG_NAME):$(IMG_TAG)),,docker-build)

smokey_agent_install: $(maybe_docker_build)
  # install network agent using helm chart using local build and wait for it to be ready
	kind load docker-image $(IMG_NAME):$(IMG_TAG)
	helm repo add honeycomb https://honeycombio.github.io/helm-charts
	OTEL_COLLECTOR_IP="$(call get_collector_ip)" \
		envsubst < smoke-tests/agent-helm-values.yaml | helm install smokey-agent honeycomb/network-agent --values -
	kubectl rollout status daemonset.apps/smokey-agent-network-agent --timeout=60s

smokey_copy_output:
  # removes the file first in case it already exists
	if [ -f ./smoke-tests/traces-orig.json ]; then \
		rm ./smoke-tests/traces-orig.json; \
	fi
  # copy collector output file to local machine to run tests on
  # the file may not be ready immediately, so retry a few times
  # note: the file is ignored in .gitignore
	for i in {1..10}; do \
		echo "attempt $$i to get collector out file"; \
		kubectl cp -c filecp default/smokey-collector-opentelemetry-collector-0:/tmp/trace.json ./smoke-tests/traces-orig.json; \
		if [ -s ./smoke-tests/traces-orig.json ]; then \
			echo "got collector out file"; \
			break; \
		fi; \
		sleep 10; \
	done

smokey_verify_output:
  # verify that the output from the collector matches the expected output
	bats ./smoke-tests/verify.bats

# A function to get the IP address of the collector service. Use = instead of := so that it is lazy-evaluated.
get_collector_ip = \
	$(shell kubectl get service smokey-collector-opentelemetry-collector --template '{{.spec.clusterIP}}')

.PHONY: smokey_echo_job
#: apply echo server and run smoke-job; not necessary for local setup - plenty of chatter already
smokey_echo_job:
	make apply-echoserver
	kubectl create --filename smoke-tests/smoke-job.yaml
	kubectl rollout status deployment.apps/echoserver --timeout=60s --namespace echoserver
	kubectl wait --for=condition=complete job/smoke-job --timeout=60s --namespace echoserver

.PHONY: unsmoke
#: teardown smoke tests
unsmoke:
	kind delete cluster

.PHONY: resmoke
#: run smoke tests again
resmoke: unsmoke smoke

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
