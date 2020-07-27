FROM golang:alpine as build-env
LABEL maintainer="daniel@widerin.net"

ENV GOBIN /go/bin

RUN mkdir /go/src/app

ADD . /go/src/app
WORKDIR /go/src/app

RUN go build -o /vnc-recorder .

FROM jrottenberg/ffmpeg:4.0-alpine
COPY --from=build-env /vnc-recorder /
ENTRYPOINT ["/vnc-recorder"]
CMD [""]
