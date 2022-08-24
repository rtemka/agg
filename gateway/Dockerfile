# syntax=docker/dockerfile:1

FROM golang:1.19-alpine3.16 as build

LABEL maintainer="Artem Rybakov<rybakov333@gmail.com>" 

WORKDIR /go/src/github.com/rtemka/agg/gateway 

COPY . .

RUN go mod tidy \
    && CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o ./gateway ./cmd/gateway.go

FROM scratch

WORKDIR /app

COPY --from=build go/src/github.com/rtemka/agg/gateway/gateway .

EXPOSE 8080

ENTRYPOINT ["./gateway"]