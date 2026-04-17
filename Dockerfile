FROM golang:1.26.1-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
COPY .gitmodules .gitmodules
COPY LLMGateway LLMGateway
COPY LLMProvider LLMProvider
COPY LLMsVerifier LLMsVerifier
COPY Models Models
COPY Challenges Challenges
RUN go mod download
COPY . .
RUN git submodule update --init --recursive 2>/dev/null || true
RUN CGO_ENABLED=1 go build -o /patreon-manager ./cmd/server
RUN CGO_ENABLED=1 go build -o /patreon-cli ./cmd/cli

FROM alpine:3.20
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /patreon-manager /app/server
COPY --from=builder /patreon-cli /app/cli
COPY --from=builder /app/internal /app/internal
COPY --from=builder /app/user /app/user
RUN mkdir -p /app/data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s CMD wget -q --spider http://localhost:8080/health || exit 1
CMD ["/app/server"]
