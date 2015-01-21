test:
	go test ./...

install:
	go install ./cmd/tsgen

examples:
	./_example/run.sh
