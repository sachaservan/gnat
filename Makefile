NAME = gnat
BIN = bin/${NAME}

all: ${BIN}

${BIN}:
	go build -o $@

run:
	go run *.go

clean:
	rm -rf ${BIN}

debug:
	GORACE="history_size=7 halt_on_error=1" go run --race *.go

install:
	go get -t -v ./...
