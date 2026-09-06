# Build context is the repository root (the bot lives in the root Go module and
# imports entities/ and game_config/):
#
#   docker build -f bot/Dockerfile -t leaderboard-bot:dev .
FROM golang:1.27-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/bot ./bot

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=builder /out/bot /usr/local/bin/bot
# Bundled scenarios; mount a ConfigMap over /scenarios to override them.
COPY bot/scenarios /scenarios

ENV BOT_SCENARIO_FILE=/scenarios/steady.yaml \
    BOT_METRICS_PORT=9100

EXPOSE 9100

ENTRYPOINT ["bot"]
