# Build context = REPO ROOT (see demo/docker-compose.yml).
FROM golang:1.23-alpine AS build
WORKDIR /src/topology
COPY linux/topology/go.mod linux/topology/go.sum ./
RUN go mod download
COPY linux/topology/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /out/topology-api ./cmd/topology-api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/topology-api /topology-api
COPY darsan/contracts/registry.yaml /registry.yaml
ENV PORT=9003 REGISTRY_PATH=/registry.yaml
EXPOSE 9003
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD ["/topology-api", "-healthcheck"]
ENTRYPOINT ["/topology-api"]
