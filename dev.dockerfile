FROM golang:1.20.3-alpine3.17

ADD . /app
WORKDIR /app

RUN apk --update --upgrade add build-base ca-certificates

ENTRYPOINT ["go", "run", "cmd/onepaas-worker/main.go"]
