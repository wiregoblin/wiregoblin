FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} go build -o /out/wiregoblin-cli ./cmd/wiregoblin-cli

FROM alpine:3.22

RUN apk add --no-cache ca-certificates

WORKDIR /workspace

COPY --from=builder /out/wiregoblin-cli /usr/local/bin/wiregoblin-cli

ENTRYPOINT ["/usr/local/bin/wiregoblin-cli"]
