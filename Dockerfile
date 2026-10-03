FROM node:22-alpine AS frontend
WORKDIR /app
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build
FROM golang:1.24-alpine AS backend
WORKDIR /app
COPY backend/go.* ./
RUN go mod download
COPY backend/ ./
RUN go test ./... && CGO_ENABLED=0 go build -o /bingo .
FROM alpine:3.21
RUN apk add --no-cache ca-certificates && adduser -D bingo
WORKDIR /app
COPY --from=backend /bingo ./bingo
COPY --from=frontend /app/dist ./static
ENV PORT=8082 STATIC_DIR=/app/static
USER bingo
EXPOSE 8082
CMD ["./bingo"]
