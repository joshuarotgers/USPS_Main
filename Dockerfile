# syntax=docker/dockerfile:1

FROM golang:1.24 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Prepare embedded OpenAPI asset
RUN mkdir -p internal/api/embedded && cp openapi/openapi.yaml internal/api/embedded/openapi.yaml
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags=embed_openapi -trimpath -ldflags "-s -w" -o /out/api ./cmd/api

FROM gcr.io/distroless/static:nonroot
WORKDIR /app
ENV PORT=8080
COPY --from=build /out/api /app/api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/api"]
