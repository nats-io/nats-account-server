FROM golang:1.12.4 AS builder

WORKDIR $GOPATH/src/github.com/nats-io/nats-account-server

MAINTAINER Waldemar Quevedo <wally@synadia.com>

COPY . .

RUN CGO_ENABLED=0 go build -v -a -tags netgo -installsuffix netgo -o /nats-account-server

FROM alpine:3.9

RUN mkdir -p /nats/bin && mkdir /nats/conf

COPY --from=builder /nats-account-server /nats/bin/nats-account-server

RUN ln -ns /nats/bin/nats-account-server /bin/nats-account-server

EXPOSE 9090

ENTRYPOINT ["/bin/nats-account-server"]
