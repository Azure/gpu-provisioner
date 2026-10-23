# Build the manager binary
FROM --platform=$BUILDPLATFORM mcr.microsoft.com/oss/go/microsoft/golang:1.26.10 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG KARPENTERVER

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
ENV GOCACHE=/root/gocache
RUN \
    --mount=type=cache,target=${GOCACHE} \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy the go source
COPY cmd/controller/main.go cmd/main.go
COPY pkg/ pkg/
COPY vendor/ vendor/

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN --mount=type=cache,target=${GOCACHE} \
    --mount=type=cache,id=gpu-provisioner-controller,sharing=locked,target=/go/pkg/mod \
    GOEXPERIMENT=nosystemcrypto CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} GO111MODULE=on go build -a -o manager -ldflags "-X sigs.k8s.io/karpenter/pkg/operator.Version=${KARPENTERVER}" cmd/main.go

# Use Azure Linux distroless as minimal base image to package the manager binary.
# Pinned by digest so the CA trust store in the image is reproducible.
FROM mcr.microsoft.com/azurelinux/distroless/minimal:3.0@sha256:c91e907c537ad861cb90bc18aece90479b0754b090afd4eb8722dd15477dc9e8
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
