NAME = gnatnode

all: ${NAME}

${NAME}:
	go build -o $@

run:
	go run *.go

clean:
	rm -rf ${NAME}

debug:
	GORACE="history_size=7 halt_on_error=1" go run --race *.go

install:
	go get -t -v ./...
