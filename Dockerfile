# syntax=docker/dockerfile:1

FROM golang:1.24 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Prepare embedded OpenAPI asset
RUN apt-get update && apt-get install -y wget ca-certificates && rm -rf /var/lib/apt/lists/* \
 && mkdir -p internal/api/embedded \
 && cp openapi/openapi.yaml internal/api/embedded/openapi.yaml \
 && wget -qO internal/api/embedded/redoc.standalone.js https://cdn.jsdelivr.net/npm/redoc@2.1.4/bundles/redoc.standalone.js \
 && wget -qO internal/api/embedded/swagger-ui-bundle.js https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui-bundle.js \
 && wget -qO internal/api/embedded/swagger-ui-standalone-preset.js https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui-standalone-preset.js \
 && wget -qO internal/api/embedded/swagger-ui.css https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui.css
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags=embed_openapi -trimpath -ldflags "-s -w -X gpsnav/internal/buildinfo.Version=${VERSION:-dev}" -o /out/api ./cmd/api

FROM gcr.io/distroless/static:nonroot
WORKDIR /app
ENV PORT=8080
COPY --from=build /out/api /app/api
# Include DB migrations for Postgres mode
COPY --from=build /src/db/migrations /app/db/migrations
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/api"]
