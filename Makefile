.DEFAULT_GOAL = all

BINARY = joe
GOARCH = amd64
GOOS = linux

VERSION?=0.1
BUILD_TIME?=$(shell date -u '+%Y%m%d-%H%M')
COMMIT?=no #$(shell git rev-parse HEAD)
BRANCH?=no #$(shell git rev-parse --abbrev-ref HEAD)

# Symlink into GOPATH
BUILD_DIR=${GOPATH}/${BINARY}

# Setup the -ldflags option for go build here, interpolate the variable values
LDFLAGS = -ldflags "-s -w \
	-X main.version=${VERSION} \
	-X main.commit=${COMMIT} \
	-X main.branch=${BRANCH}\
	-X main.buildTime=${BUILD_TIME}"

# Go tooling command aliases
GOBUILD = GO111MODULE=on CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build ${LDFLAGS}
GOTEST = GO111MODULE=on go test
GORUN = GO111MODULE=on go run ${LDFLAGS}


# Build the project
all: clean vet build

# Install the linter to $GOPATH/bin which is expected to be in $PATH
install-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.22.2

run-lint:
	golangci-lint run

lint: install-lint run-lint

build:
	${GOBUILD} -o bin/${BINARY} ./cmd/joe/main.go

build-ee:
	${GOBUILD} -tags ee -o bin/${BINARY} ./cmd/joe/main.go

test:
	go test ./pkg/...

vet:
	go vet ./...

fmt:
	go fmt $$(go list ./... | grep -v /vendor/)

clean:
	-rm -f bin/*

run:
	go run ${LDFLAGS} ./cmd/joe/main.go

.PHONY: all main test vet fmt clean run

