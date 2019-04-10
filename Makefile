
build: fmt check compile

fmt:
	misspell -locale US .
	gofmt -s -w nats-account-server/*.go
	gofmt -s -w nats-account-server/conf/*.go
	gofmt -s -w nats-account-server/core/*.go
	gofmt -s -w nats-account-server/logging/*.go
	goimports -w nats-account-server/*.go
	goimports -w nats-account-server/conf/*.go
	goimports -w nats-account-server/core/*.go
	goimports -w nats-account-server/logging/*.go

check:
	go vet ./...
	staticcheck ./...

update:
	go get -u honnef.co/go/tools/cmd/staticcheck
	go get -u github.com/client9/misspell/cmd/misspell
  
compile:
	go build ./...

cover: test
	go tool cover -html=./coverage.out

test: check
	rm -rf ./cover.out
	go test -coverpkg=./... -coverprofile=./cover.out ./...