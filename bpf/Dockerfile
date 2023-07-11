FROM golang:1.20
RUN apt update -yq && apt install -yq clang llvm make
VOLUME [ "/src" ]
WORKDIR /src
CMD [ "make", "generate" ]