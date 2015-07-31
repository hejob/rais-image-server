GO_NAMESPACE_DIR=$(GOPATH)/src/github.com/uoregon-libraries
GO_PROJECT_SYMLINK=$(GO_NAMESPACE_DIR)/rais-image-server
SYMLINK_EXISTS=$(GO_PROJECT_SYMLINK)/Makefile
GO_PROJECT_NAME=github.com/uoregon-libraries/rais-image-server
GOBIN=$(GOROOT)/bin/go

# Dependencies
IMGRESIZEDEP=github.com/nfnt/resize
IMGRESIZE=$(GOPATH)/src/$(IMGRESIZEDEP)
IMGTIFFDEP=golang.org/x/image/tiff
IMGTIFF=$(GOPATH)/src/$(IMGTIFFDEP)

# Makefile directory
MakefileDir := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))

# All library files contribute to the need to recompile (except tests!  How do
# we skip those?)
SRCS := openjpeg/*.go iiif/*.go magick/*.go openjpeg/*.[ch] magick/*.[ch]

.PHONY: all generate binaries test format lint clean distclean docker

# Default target builds binaries
all: binaries

# Generated code
generate: transform/rotation.go

transform/rotation.go: transform/generator.go transform/template.txt
	$(GOBIN) run transform/generator.go
	gofmt -l -w -s transform/rotation.go

# Dependency-getters
deps: $(IMGRESIZE) $(IMGTIFF)
$(IMGRESIZE):
	$(GOBIN) get $(IMGRESIZEDEP)
$(IMGTIFF):
	$(GOBIN) get $(IMGTIFFDEP)

# dir/symlink creation - mandatory for any binary building to work
#
# We use symlink/main.go to avoid the symlink being a dependency - *any* change
# to directory listing will cause make to think it needs a rebuild if the rule
# is just the symlink itself
$(SYMLINK_EXISTS):
	mkdir -p $(GO_NAMESPACE_DIR)
	ln -s $(CURDIR) $(GO_PROJECT_SYMLINK)

# Binary building rules
binaries: bin/rais-server
bin/rais-server: $(SYMLINK_EXISTS) $(IMGRESIZE) $(SRCS) cmd/rais-server/*.go transform/rotation.go
	$(GOBIN) build -tags jp2 -o bin/rais-server ./cmd/rais-server

# Testing
test: $(SYMLINK_EXISTS) $(IMGRESIZE)
	$(GOBIN) test -tags jp2 ./openjpeg ./cmd/rais-server ./iiif ./fakehttp

format:
	find . -name "*.go" | xargs gofmt -l -w -s

lint:
	golint ./...

# Cleanup
clean:
	rm -f bin/*
	rm -f transform/rotation.go

distclean: clean
	rm -f $(GO_PROJECT_SYMLINK)
	rmdir --ignore-fail-on-non-empty $(GO_NAMESPACE_DIR)

# Rebuild docker containers
docker:
	docker stop rais || true
	docker rm rais || true
	docker build --rm -t uolibraries/rais:libs -f docker/Dockerfile.libs $(MakefileDir)
	docker build --rm -t uolibraries/rais:build -f docker/Dockerfile.build $(MakefileDir)
	mkdir -p $(MakefileDir)/docker/bin
	docker run --rm -it -v $(MakefileDir)/docker/bin:/tmp/hostbin uolibraries/rais:build cp /tmp/go/bin/rais-server /tmp/hostbin
	docker build --rm -t uolibraries/rais -f docker/Dockerfile.prod $(MakefileDir)
	rm -rf $(MakefileDir)/docker/bin
