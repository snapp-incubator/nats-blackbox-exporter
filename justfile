default:
    @just --list

# build nats-blackbox-exporter binary
build:
    go build -o nats-blackbox-exporter ./cmd/nats-blackbox-exporter

# update go packages
update:
    @cd ./cmd/nats-blackbox-exporter && go get -u

# run tests
test:
    go test -v ./... -covermode=atomic -coverprofile=coverage.out

# run golangci-lint
lint:
    golangci-lint run -c .golangci.yml
