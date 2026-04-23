# Multi-stage build producing a single container image that serves both
# the React SPA (built assets) and the Go API on a single port.
#
# Stage 1: build the frontend SPA.
FROM node:22-alpine AS frontend
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend ./
# VITE_API_URL defaults to "" at build time so the SPA calls the same
# origin as the host page. Override via --build-arg for split deploys.
ARG VITE_API_URL=""
ARG VITE_STRIPE_PUBLISHABLE_KEY=""
ENV VITE_API_URL=$VITE_API_URL
ENV VITE_STRIPE_PUBLISHABLE_KEY=$VITE_STRIPE_PUBLISHABLE_KEY
RUN npm run build

# Stage 2: build the Go backend (CGO disabled — modernc.org/sqlite is pure Go).
FROM golang:1.25-alpine AS backend
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend ./
ENV CGO_ENABLED=0 GOOS=linux GOFLAGS="-trimpath"
RUN go build -ldflags "-s -w" -o /out/nast-backend .

# Stage 3: minimal runtime. Distroless is smaller but alpine eases debugging;
# switch to gcr.io/distroless/static-debian12 if you never exec into it.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S nast && adduser -S nast -G nast && \
    mkdir -p /data && chown nast:nast /data
WORKDIR /app
COPY --from=backend /out/nast-backend /app/nast-backend
COPY --from=frontend /app/dist /app/web
USER nast
ENV PORT=8080 \
    DATABASE_PATH=/data/nast.db \
    STATIC_DIR=/app/web
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q -O- http://127.0.0.1:8080/api/health || exit 1
ENTRYPOINT ["/app/nast-backend"]
