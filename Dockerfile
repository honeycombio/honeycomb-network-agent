FROM golang:1.20 as builder
RUN apt update -yq && apt install -yq clang llvm make
WORKDIR /src
COPY . .
RUN make build

FROM alpine:3.14
COPY --from=builder /src/hny-ebpf-agent /bin/hny-ebpf-agent
CMD [ "/bin/hny-ebpf-agent" ]