FROM golang:1.20 as builder
RUN apt update -yq && apt install -yq clang llvm make
WORKDIR /src
COPY . .
RUN make build

FROM ubuntu:22.04
RUN apt-get update -yq && apt-get install -yq ca-certificates
COPY --from=builder /src/hny-ebpf-agent /bin/hny-ebpf-agent
CMD [ "/bin/hny-ebpf-agent" ]