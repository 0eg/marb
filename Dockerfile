FROM golang:alpine AS builder

WORKDIR /app
COPY . .
RUN go build -o /bin/marb

FROM alpine
COPY --from=builder /bin/marb /bin/marb
RUN mkdir -p /var/www
