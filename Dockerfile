#build stage
FROM golang:alpine AS builder
RUN apk add --no-cache git gpgme-dev gcc musl-dev 
WORKDIR /go/src/app
COPY . .
RUN go get -v ./...
RUN go build -o /go/bin/app -v -tags "exclude_graphdriver_devicemapper exclude_graphdriver_btrfs remote" containermon.go

#final stage
FROM alpine:latest
ENV SOCKET_FILE_PATH=''
ENV HEALTH_CHECK_URL=''
ENV CRON_SCHEDULER_CONFIG=''
RUN apk --no-cache add ca-certificates gpgme
COPY --from=builder /go/bin/app /app
ENTRYPOINT ["/bin/sh", "-c", "/app"]
LABEL Name=containermon Version=1.0