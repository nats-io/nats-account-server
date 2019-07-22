
build: fmt check compile

fmt:
	#misspell -locale US .
	gofmt -s -w server/conf/*.go
	gofmt -s -w server/core/*.go
	gofmt -s -w server/logging/*.go
	gofmt -s -w server/store/*.go
	goimports -w server/conf/*.go
	goimports -w server/core/*.go
	goimports -w server/logging/*.go
	goimports -w server/store/*.go

check:
	go vet ./...
	staticcheck ./...

update:
	go get -u honnef.co/go/tools/cmd/staticcheck
	go get -u github.com/client9/misspell/cmd/misspell
  
compile:
	go build ./...

install: build
	go install ./...

cover: test
	go tool cover -html=./coverage.out

test: check
	go mod vendor
	rm -rf ./cover.out
	go test -tags test -race -coverpkg=./server/... -coverprofile=./coverage.out ./...

fasttest:
	scripts/cov.sh

failfast:
	go test -tags test -race -failfast ./...