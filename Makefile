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
smoke: #docker-build
	kind create cluster

  # install opentelemetry collector
	helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts
	helm install smokey-collector open-telemetry/opentelemetry-collector --values smoke-tests/collector-helm-values.yaml

  # install network agent using helm chart and local build
	kind load docker-image $(IMG_NAME):$(IMG_TAG)
	helm repo add honeycomb https://honeycombio.github.io/helm-charts
	helm install smokey-agent honeycomb/network-agent --values smoke-tests/agent-helm-values.yaml

  # wait for collector and agent to be ready
	kubectl rollout status statefulset.apps/smokey-collector-opentelemetry-collector --timeout=60s
	kubectl rollout status daemonset.apps/smokey-agent-network-agent --timeout=10s

.PHONY: save-for-later
#: apply echo server and run smoke-job; not necessary for local setup - plenty of chatter already
save-for-later:
	make apply-echoserver
	kubectl create --filename smoke-tests/smoke-job.yaml
	kubectl rollout status deployment.apps/echoserver --timeout=10s --namespace echoserver
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
