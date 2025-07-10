GOPATH:=$(shell go env GOPATH)

.PHONY: init
init:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install github.com/micro/micro/v5/cmd/protoc-gen-micro@latest
	go install github.com/micro/micro/v5/cmd/protoc-gen-openapi@latest

.PHONY: api
api:
	protoc --openapi_out=. --proto_path=. proto/carts.proto

.PHONY: proto
proto:
	protoc --proto_path=. --micro_out=. --go_out=:. proto/carts.proto

.PHONY: build
build:
	go build -o carts *.go

.PHONY: test
test:
	go test -v ./... -cover

.PHONY: docker
docker:
	docker build . -t carts:latest

.PHONY: ent new-entity
ent:
	echo "Generating Entgo client code ..."
	go generate ./ent

# Create new entities (usage: make new-entity entities="User Product Order")
new-entity:
	@if [ -z "$(entities)" ]; then \
		echo "Error: Please specify entities (e.g., make new-entity entities=\"User Product Order\")"; \
		exit 1; \
	fi
	@echo "Creating new Entgo entities: $(entities)"
	@for entity in $(entities); do \
		echo "Creating $$entity..."; \
		go run -mod=mod entgo.io/ent/cmd/ent new $$entity; \
	done