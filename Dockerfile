FROM golang:1.20 as builder
RUN apt-get update && apt-get install -yq \
    clang \
    llvm \
    libpcap-dev \
    flex \
    bison

# Install PF_RING
WORKDIR /app
RUN curl https://github.com/ntop/PF_RING/archive/refs/tags/8.4.0.tar.gz -Lo ./pfring.tar.xz && \
    tar -xf pfring.tar.xz && \
    mv ./PF_RING-* ./pfring
WORKDIR /app/pfring/userland/lib
RUN ./configure --host --host-alias && \
    make && \
    make install
WORKDIR /app/pfring
RUN cp ./kernel/linux/pf_ring.h /usr/include/linux
RUN cp /usr/local/lib/libpfring.so.8.4.0 /usr/lib/libpfring.so.8

WORKDIR /src
COPY go.* .
RUN go mod download
COPY . .
RUN make build
# ENTRYPOINT [ "/src/hny-ebpf-agent" ]

# FROM debian:10-slim
# RUN apt-get update -yq && apt-get install -yq ca-certificates libpcap-dev
# COPY --from=builder /usr/local/lib/libpfring.so.8.4.0 /usr/lib/libpfring.so.8
# COPY --from=builder /src/hny-ebpf-agent /bin/hny-ebpf-agent
# ENTRYPOINT [ "/bin/hny-ebpf-agent" ]