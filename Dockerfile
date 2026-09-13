FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o leaderboard-engine .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/leaderboard-engine /usr/local/bin/leaderboard-engine

EXPOSE 3000

ENTRYPOINT ["leaderboard-engine"]
