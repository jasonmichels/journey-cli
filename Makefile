build-linux:
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o journey-cli .
test:
	go test -v ./... -bench . -cover