# API server — small static binary (Phase 3+).
# At Phase 1 this is a stub but the image still builds.

FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api-server ./cmd/api-server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api-server /api-server
EXPOSE 7000
ENTRYPOINT ["/api-server"]
