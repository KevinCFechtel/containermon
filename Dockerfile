#build stage
FROM golang:alpine AS builder
RUN apk add --no-cache git
WORKDIR /go/src/app
COPY . .
RUN go get -d -v ./...
RUN go build  -o /go/bin/containermon -v containermon.go

#final stage
FROM alpine:latest
ENV SOCKET_FILE_PATH=NoConfigFile
ENV HEALTH_CHECK_URL=''
RUN apk --no-cache add ca-certificates
COPY --from=builder /go/bin/containermon /containermon
ENTRYPOINT ["/bin/sh", "-c", "/containermon -socketPath=$SOCKET_FILE_PATH -healthcheckUrl=$HEALTH_CHECK_URL"]
LABEL Name=goRSSDedup Version=1.0