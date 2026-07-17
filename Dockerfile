FROM golang:1.26 AS build-env

# Add dependencies for building and for eBPF code generation
# llvm is a dependency for clang
# libbpf-dev provides the C headers for libbpf
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    clang \
    llvm \
    libelf-dev \
    bpftool \
    libbpf-dev \
    bpfcc-tools \
    linux-libc-dev-amd64-cross \
    libc6-dev-i386-cross \
    linux-headers-amd64

# Add source code
ADD . /src
WORKDIR /src


# Configure git for private repositories
ARG GIT_TOKEN
ENV GOPRIVATE=dev.azure.com
RUN echo ${GIT_TOKEN}
RUN git config --global url."https://${GIT_TOKEN}@dev.azure.com/bloopi/bloopi/_git/shared_models".insteadOf "https://dev.azure.com/bloopi/bloopi/_git/shared_models"
RUN go env -w GOPRIVATE=dev.azure.com


# Generate eBPF Go files. This requires kernel headers (BTF).
# First, generate vmlinux.h from the running kernel's BTF info.
# Note: This requires the build environment to have access to /sys/kernel/btf/vmlinux
RUN mkdir -p internal/cloud/flows/headers && \
    bpftool btf dump file /sys/kernel/btf/vmlinux format c > internal/cloud/flows/headers/vmlinux.h

# Now, run go generate which uses the header file created above.
# The `generate.go` file will also clean up the .c file afterwards.
RUN go generate ./...

# Build the final Go binary
RUN CGO_ENABLED=0 go build -a -o cmd/agent/agent cmd/agent/main.go

# --- Final Stage ---
FROM alpine:3.21.6

COPY --from=build-env /src/cmd/agent/agent /agent

RUN addgroup -S coordimap-agent && adduser -S coordimap-agent -G coordimap-agent
USER coordimap-agent

CMD [ "/agent" ]
