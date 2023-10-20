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
FROM redhat/ubi9-minimal
# install libpcap-devel and clean up after to help reduce image size
RUN microdnf -y update && \
    microdnf -y install libpcap-devel && \
    microdnf -y clean all
# link libpcap.so.0.8 to libpcap.so to the agent can find it
RUN ln -s /usr/lib64/libpcap.so /usr/lib64/libpcap.so.0.8
COPY --from=base /src/hny-network-agent /bin/hny-network-agent
ENTRYPOINT [ "/bin/hny-network-agent" ]
