FROM golang:1.17 AS builder

WORKDIR /src/nats-account-server

LABEL maintainer "Waldemar Quevedo <wally@nats.io>"
LABEL maintainer "Jaime Piña <jaime@nats.io>"
LABEL maintainer "Stephen Asbury <sasbury@nats.io>"

COPY . .

RUN go mod download
RUN CGO_ENABLED=0 go build -v -a -tags netgo -installsuffix netgo -o /nats-account-server

FROM alpine

RUN apk add --update ca-certificates && mkdir -p /nats/bin && mkdir /nats/conf

COPY --from=builder /nats-account-server /nats/bin/nats-account-server

RUN ln -ns /nats/bin/nats-account-server /bin/nats-account-server

ENTRYPOINT ["/bin/nats-account-server"]
