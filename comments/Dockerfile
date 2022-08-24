# syntax=docker/dockerfile:1

FROM golang:1.19 As build
LABEL maintainer="Artem Rybakov<rybakov333@gmail.com>" 

WORKDIR /go/src/github.com/rtemka/agg/comments

COPY . .

RUN go mod tidy \ 
    && CC=$(which musl-gcc) go build -trimpath --ldflags '-s -w -linkmode external -extldflags "-static"' -o ./comments ./cmd/comments.go

FROM scratch

WORKDIR /app

COPY --from=build go/src/github.com/rtemka/agg/comments/comments .
COPY --from=build go/src/github.com/rtemka/agg/comments/comments.sql .

EXPOSE 8080

ENTRYPOINT ["./comments"]