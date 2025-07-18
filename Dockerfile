# syntax=docker/dockerfile:1
FROM golang:1.21-alpine as builder
WORKDIR /app
COPY . .
RUN go mod tidy && go build -o gwa-b01 main.go

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/gwa-b01 .
EXPOSE 8080
CMD ["./gwa-b01"]
