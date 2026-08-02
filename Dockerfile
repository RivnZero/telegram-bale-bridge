# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILT=unknown
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w \
      -X main.version=${VERSION} \
      -X main.commit=${COMMIT} \
      -X main.built=${BUILT}" \
    -o /out/bridge ./cmd/bridge

FROM alpine:3.21
RUN apk add --no-cache ca-certificates \
    && adduser -D -u 10001 bridgeuser
COPY --from=build /out/bridge /usr/local/bin/bridge
USER bridgeuser
ENV DATABASE_PATH=/app/data/bridge.db \
    TEMP_DIRECTORY=/app/data/tmp
VOLUME ["/app/data"]
ENTRYPOINT ["bridge"]
