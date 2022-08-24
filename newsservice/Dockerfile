# syntax=docker/dockerfile:1

FROM golang:1.19-alpine3.16 As build
LABEL maintainer="Artem Rybakov<rybakov333@gmail.com>" 

WORKDIR /go/src/github.com/rtemka/agg/news

# copy source code to WORKDIR
COPY . .

# install dependencies

RUN go mod tidy

# build binary; CGO_ENABLED=0 needed to compile binary with no external dependencies
# -ldflags "-s -w" strips out debugging information from binary

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o ./cmd/news/news ./cmd/news/news.go

# Second stage

FROM alpine:3.16.1

WORKDIR /app

COPY --from=build go/src/github.com/rtemka/agg/news/cmd/news/ .

# 8080 - API listen port

EXPOSE 8080

ENTRYPOINT [ "./news", "./config.json" ]