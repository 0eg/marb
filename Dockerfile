FROM golang:alpine AS builder

WORKDIR /app
COPY main.go go.mod .
RUN go build -o /bin/marb

FROM alpine
COPY --from=builder /bin/marb /bin/marb
RUN mkdir -p /var/www
