FROM golang:1.20 as builder
RUN apt update -yq && apt install -yq clang llvm make libpcap-dev
WORKDIR /src
COPY go.* .
RUN go mod download
COPY . .
RUN make build

FROM ubuntu:22.04
RUN apt-get update -yq && apt-get install -yq ca-certificates libpcap-dev
COPY --from=builder /src/hny-ebpf-agent /bin/hny-ebpf-agent
ENTRYPOINT [ "/bin/hny-ebpf-agent" ]