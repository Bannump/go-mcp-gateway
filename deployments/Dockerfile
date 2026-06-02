# Stage 1: build
# Note: if SQLite support is needed (STORAGE_BACKEND=sqlite), switch final image
# to gcr.io/distroless/base-debian12 and remove CGO_ENABLED=0 so CGO is enabled.
FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server ./cmd/server

# Stage 2: minimal runtime image
FROM gcr.io/distroless/static-debian12

COPY --from=builder /server /server

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]
