BINARY_NAME=aws-ssm-connect
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-s -w -X main.version=${VERSION}"

build:
	go build ${LDFLAGS} -o ${BINARY_NAME} .

clean:
	go clean
	rm -f ${BINARY_NAME}

install:
	go install ${LDFLAGS}

release:
	goreleaser release --snapshot --clean

run:
	go run ${LDFLAGS} . $(ARGS)