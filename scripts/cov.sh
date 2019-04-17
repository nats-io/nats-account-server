#!/bin/bash -e

rm -rf ./cov
mkdir cov
go test -covermode=atomic -coverprofile=./cov/conf.out ./nats-account-server/conf
go test -covermode=atomic -coverprofile=./cov/core.out ./nats-account-server/core
go test -covermode=atomic -coverprofile=./cov/logging.out ./nats-account-server/logging
go test -covermode=atomic -coverprofile=./cov/store.out ./nats-account-server/store

gocovmerge ./cov/*.out > ./coverage.out
rm -rf ./cov

# If we have an arg, assume travis run and push to coveralls. Otherwise launch browser results
if [[ -n $1 ]]; then
    $HOME/gopath/bin/goveralls -coverprofile=coverage.out -service travis-ci
    rm -rf ./coverage.out
else
    go tool cover -html=coverage.out
fi