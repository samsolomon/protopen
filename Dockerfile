# Stage 1: Build frontend
FROM node:20-alpine AS frontend
WORKDIR /app
COPY package.json package-lock.json ./
COPY web/package.json ./web/
RUN npm ci --workspace=web
COPY web/ ./web/
RUN npm run --workspace=web build

# Stage 2: Build Go binary
FROM golang:1.25-alpine AS backend
WORKDIR /app/api
COPY api/go.mod api/go.sum ./
RUN go mod download
COPY api/ ./
COPY --from=frontend /app/web/dist ./dist/
RUN CGO_ENABLED=0 go build -o /protopen .

# Stage 3: Runtime
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
RUN adduser -D protopen
RUN mkdir -p /home/protopen/.data/ingest && chown -R protopen:protopen /home/protopen
COPY --from=backend /protopen /protopen
USER protopen
WORKDIR /home/protopen
EXPOSE 8080
CMD ["/protopen"]
