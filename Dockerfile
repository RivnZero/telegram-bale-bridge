# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bridge ./cmd/bridge

FROM alpine:3.21
RUN apk add --no-cache ca-certificates \
    && adduser -D -u 10001 bridgeuser
COPY --from=build /out/bridge /usr/local/bin/bridge
USER bridgeuser
ENV DATABASE_PATH=/app/data/bridge.db \
    TEMP_DIRECTORY=/app/data/tmp
VOLUME ["/app/data"]
ENTRYPOINT ["bridge"]
