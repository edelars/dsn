APP = device-status-aggregator
.PHONY: build swagger


#TESTS
test:
	go test -mod=vendor ./...

race:
	go test -mod=vendor ./... -race

cover:
	go test -mod=vendor ./... -coverpkg=./pkg/... -coverprofile coverage.out -covermode count
	go tool cover -func=coverage.out
