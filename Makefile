# Makefile directory
MakefileDir := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))

.PHONY: all generate binaries test format lint clean distclean docker plugins

# Default target builds binaries
all: binaries

# Generated code
generate: src/transform/rotation.go

src/transform/rotation.go: src/transform/generator.go src/transform/template.txt
	go run src/transform/generator.go
	gofmt -l -w -s src/transform/rotation.go

# Binary building rules
binaries: src/transform/rotation.go
	go build -o ./bin/rais-server rais/src/cmd/rais-server
	go build -o ./bin/jp2info rais/src/cmd/jp2info

# Testing
test:
	go test rais/src/...

bench:
	go test -bench=. -benchtime=5s -count=2 rais/src/openjpeg rais/src/cmd/rais-server

format:
	find src/ -name "*.go" | xargs gofmt -l -w -s

lint:
	golint src/...

# Cleanup
clean:
	rm -rf bin/
	rm -rf pkg/
	rm -f src/transform/rotation.go

# (Re)build the separated docker containers
docker:
	docker pull uolibraries/rais
	docker-compose build rais-build
	docker-compose run --rm rais-build make clean
	docker-compose run --rm rais-build make
	docker build --rm -t uolibraries/rais:f28 -f docker/Dockerfile.prod $(MakefileDir)

plugins:
	go build -buildmode=plugin -o bin/plugins/external-images.so rais/src/plugins/external-images
