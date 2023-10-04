# uses docker multi-stage builds: https://docs.docker.com/build/building/multi-stage/
# base stage builds the agent binary for use in later stages
FROM golang:1.21 as base
RUN apt update -yq && \
    apt install -yq make libpcap-dev
WORKDIR /src
COPY go.* .
RUN go mod download
COPY . .
RUN make build

# run tests with 'docker build --target test .'; skips the runnable image build
FROM base as test
RUN make test

# last unnamed stage is the default target for any image build
# this produces the runnable agent image
# the --no-install-recommends flag is used to avoid installing unnecessary packages
FROM ubuntu:22.04
RUN apt-get update -yq && \
    apt-get install -yq --no-install-recommends \
    ca-certificates libpcap-dev
COPY --from=base /src/hny-network-agent /bin/hny-network-agent
ENTRYPOINT [ "/bin/hny-network-agent" ]
