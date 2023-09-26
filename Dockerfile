FROM golang:1.21 as base
RUN apt update -yq && apt install -yq make libpcap-dev
WORKDIR /src
COPY go.* .
RUN go mod download
COPY . .
RUN make build

FROM ubuntu:22.04 as build
RUN apt-get update -yq && apt-get install -yq ca-certificates libpcap-dev
COPY --from=base /src/hny-network-agent /bin/hny-network-agent
ENTRYPOINT [ "/bin/hny-network-agent" ]

FROM base as test
RUN make test
