# Build the manager binary
FROM golang:1.22.4 AS builder

ARG GIT_COMMIT=dev
ARG GIT_BRANCH=dev
ARG TARGETOS
ARG TARGETARCH

WORKDIR $GOPATH/openperouter
# Cache the downloads
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

RUN --mount=type=cache,target=/root/.cache/go-build \
  --mount=type=cache,target=/go/pkg \
  CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -v -o reloader cmd/reloader/main.go \
  && \
  CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -v -o controller cmd/hostcontroller/main.go \
  && \
  CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -v -o cp-tool cmd/cp-tool/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
#FROM gcr.io/distroless/static:nonroot
FROM fedora:latest
WORKDIR /
RUN dnf install -y jq crictl iproute
COPY --from=builder /go/openperouter/reloader .
COPY --from=builder /go/openperouter/controller .
COPY --from=builder /go/openperouter/cp-tool .
USER 65532:65532

ENTRYPOINT ["/controller"]
