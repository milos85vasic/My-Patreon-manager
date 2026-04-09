FROM golang:1.26.1-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /patreon-manager ./cmd/server
RUN CGO_ENABLED=0 go build -o /patreon-cli ./cmd/cli

FROM alpine:3.20
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /patreon-manager /app/server
COPY --from=builder /patreon-cli /app/cli
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s CMD wget -q --spider http://localhost:8080/health || exit 1
CMD ["/app/server"]
