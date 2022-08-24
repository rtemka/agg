# syntax=docker/dockerfile:1

FROM golang:1.19-alpine3.16 As build
LABEL maintainer="Artem Rybakov<rybakov333@gmail.com>" 

WORKDIR /go/src/github.com/rtemka/agg/commscheck

COPY . .

RUN go mod tidy \
    && CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o ./commscheck ./... 

FROM scratch

WORKDIR /app

COPY --from=build go/src/github.com/rtemka/agg/commscheck/commscheck .

EXPOSE 8080

ENTRYPOINT ["./commscheck"]