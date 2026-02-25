# ============================================================================
# Stage 1: Build the React dashboard
# ============================================================================
FROM node:22-alpine AS dashboard-builder

WORKDIR /build/dashboard
COPY dashboard/package.json dashboard/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY dashboard/ ./
RUN npm run build

# ============================================================================
# Stage 2: Build the Go binary
# ============================================================================
FROM golang:1.25-alpine AS go-builder

# CGO is required for mattn/go-sqlite3
RUN apk add --no-cache gcc musl-dev

WORKDIR /build

# Cache Go module downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY internal/ internal/

# Copy built dashboard into the embed directory
COPY --from=dashboard-builder /build/dashboard/dist/ internal/dashboard/dist/

# Build args for version info
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=1 go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" \
    -o /agentwarden \
    ./cmd/agentwarden

# ============================================================================
# Stage 3: Minimal runtime image
# ============================================================================
FROM alpine:3.21

# SQLite needs libc (provided by alpine base), and ca-certs for HTTPS proxying
RUN apk add --no-cache ca-certificates tzdata

COPY --from=go-builder /agentwarden /usr/local/bin/agentwarden

# Create data directory for SQLite storage
RUN mkdir -p /data

EXPOSE 6777

ENTRYPOINT ["agentwarden"]
CMD ["start", "--config", "/etc/agentwarden/config.yaml"]
