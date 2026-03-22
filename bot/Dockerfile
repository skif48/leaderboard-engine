FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY bot/main.go .
RUN go mod init bot && CGO_ENABLED=0 go build -o bot .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/bot /usr/local/bin/bot
COPY game_config/game_config.json /app/game_config.json

WORKDIR /app

ENTRYPOINT ["bot"]
