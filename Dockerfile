# syntax=docker/dockerfile:1.7

FROM node:26-alpine@sha256:6bd9ab43c0ec0190ca7b2d193906cb8b7edf6e7f4afa43ddeccd6452710bffb5 AS web
WORKDIR /web
COPY console/web/package.json console/web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY console/web/ ./
RUN npm run build

FROM golang:1.27.3@sha256:e519d3b0a5b285e5aa53ab9eef02bc6bbb9cb04608389b743a136a9ce75f0d1e AS build
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

FROM gcr.io/distroless/static-debian13:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931 AS testmcp
COPY --from=build /out/testmcp /usr/local/bin/testmcp
ENTRYPOINT ["/usr/local/bin/testmcp"]

FROM gcr.io/distroless/static-debian13:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931
COPY --from=build --chown=65532:65532 /dirs/ /
COPY --from=build /out/connector-engine /out/cnctl /out/connector-console /usr/local/bin/
USER 65532:65532
EXPOSE 8080 8081 7465
ENTRYPOINT ["/usr/local/bin/connector-engine"]
