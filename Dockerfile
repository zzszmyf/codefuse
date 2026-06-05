# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o codefuse ./cmd/codefuse

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates
WORKDIR /workspace

COPY --from=builder /build/codefuse /usr/local/bin/codefuse

ENTRYPOINT ["codefuse"]
CMD ["--help"]
