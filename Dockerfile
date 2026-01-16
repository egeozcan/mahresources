# Stage 1: Build frontend assets
FROM node:20-alpine AS frontend-builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY src/ ./src/
COPY index.css vite.config.js postcss.config.js ./
RUN npm run build-css && npm run build-js

# Stage 2: Build Go binary
FROM golang:1.21-alpine AS go-builder
RUN apk add --no-cache gcc musl-dev sqlite-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/public/dist ./public/dist
COPY --from=frontend-builder /app/public/tailwind.css ./public/tailwind.css
RUN CGO_ENABLED=1 go build --tags 'json1 fts5' -o mahresources

# Stage 3: Runtime
FROM alpine:3.19
RUN apk add --no-cache sqlite-libs ca-certificates
WORKDIR /app
COPY --from=go-builder /app/mahresources .
COPY --from=go-builder /app/templates ./templates
COPY --from=go-builder /app/public ./public
RUN mkdir -p /app/data /app/files
ENV DB_TYPE=SQLITE
ENV DB_DSN=/app/data/test.db
ENV FILE_SAVE_PATH=/app/files
ENV BIND_ADDRESS=0.0.0.0:8181
ENV SKIP_FTS=1
EXPOSE 8181
CMD ["./mahresources"]
