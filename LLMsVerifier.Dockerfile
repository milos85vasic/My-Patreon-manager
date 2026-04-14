# LLMsVerifier build for My-Patreon-Manager
# Builds from the LLMsVerifier submodule + required dependency submodules.
# The llm-verifier Go module has replace directives:
#   digital.vasic.llmprovider => ../../LLMProvider
#   digital.vasic.models      => ../Models       (via LLMProvider)
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata gcc musl-dev

WORKDIR /src

# Copy the llm-verifier module (the actual server code)
COPY LLMsVerifier/llm-verifier/ ./llm-verifier/

# Copy dependency submodules to match the replace directive paths.
# llm-verifier/go.mod: replace digital.vasic.llmprovider => ../../LLMProvider
# LLMProvider/go.mod:  replace digital.vasic.models => ../Models
COPY LLMProvider/ ./LLMProvider/
COPY Models/ ./Models/

# Fix the replace directives to match our flat layout
WORKDIR /src/llm-verifier
RUN sed -i 's|=> ../../LLMProvider|=> ../LLMProvider|g' go.mod
RUN go mod tidy || true
RUN CGO_ENABLED=1 GOOS=linux go build -o /llm-verifier ./cmd

# Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata wget libc6-compat && \
    addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

WORKDIR /app
COPY --from=builder /llm-verifier .
COPY --from=builder /src/llm-verifier/config.yaml.example ./config.yaml
RUN mkdir -p /app/data && chown -R appuser:appgroup /app

USER appuser

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=10s --retries=3 --start-period=40s \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["./llm-verifier", "server"]
