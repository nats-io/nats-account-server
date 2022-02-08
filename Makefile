
build: fmt check compile

fmt:
	goimports -w main.go
	goimports -w server/conf/*.go
	goimports -w server/core/*.go
	goimports -w server/store/*.go

	go mod tidy

check:
	go vet ./...
	staticcheck ./...

update:
	go get -u honnef.co/go/tools/cmd/staticcheck

compile:
	go build ./...

releaser:
	goreleaser --snapshot --rm-dist --skip-validate --skip-publish --parallelism 12

install: build
	go install ./...

cover: test
	go tool cover -html=./coverage.out


test: fmt check
	rm -rf ./cover.out
	go test -tags test -race -coverpkg=./server/... -coverprofile=./coverage.out ./...

fasttest:
	scripts/cov.sh

failfast:
	go test -tags test -race -failfast ./...

.PHONY: dockerx
dockerx:
ifneq ($(ver),)
	# Ensure 'docker buildx ls' shows correct platforms.
	docker buildx build \
		--tag natsio/nats-account-server:$(ver) --tag natsio/nats-account-server:latest \
		--platform linux/amd64,linux/arm/v6,linux/arm/v7,linux/arm64/v8 \
		--push .
else
	# Missing version, try this.
	# make dockerx ver=1.2.3
	exit 1
endif
