# syntax=docker/dockerfile:1.7

FROM node:26-alpine AS web
WORKDIR /web
COPY console/web/package.json console/web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY console/web/ ./
RUN npm run build

FROM golang:1.27.3 AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
COPY go.mod go.sum ./
COPY . .
COPY --from=web /web/dist/app ./console/web/dist/app
ARG VERSION=dev
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg/mod \
    go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/connector-engine ./cmd/connector-engine && \
    go build -ldflags "-s -w" -o /out/cnctl ./cmd/cnctl && \
    go build -ldflags "-s -w" -o /out/connector-console ./cmd/connector-console && \
    go build -ldflags "-s -w" -o /out/testmcp ./console/testmcp && \
    mkdir -p /dirs/var/lib/connector-engine/secrets /dirs/var/lib/connector-console /dirs/run/connector-engine

FROM gcr.io/distroless/static-debian13:nonroot AS testmcp
COPY --from=build /out/testmcp /usr/local/bin/testmcp
ENTRYPOINT ["/usr/local/bin/testmcp"]

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build --chown=65532:65532 /dirs/ /
COPY --from=build /out/connector-engine /out/cnctl /out/connector-console /usr/local/bin/
USER 65532:65532
EXPOSE 8080 8081 7465
ENTRYPOINT ["/usr/local/bin/connector-engine"]
