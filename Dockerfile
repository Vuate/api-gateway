FROM golang:1.26.1-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o api-gateway ./cmd/main.go

FROM alpine:3.23
WORKDIR /root/
COPY --from=builder /app/api-gateway .
COPY --from=builder /app/docs ./docs
EXPOSE 9000
CMD ["./api-gateway"]
