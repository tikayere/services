
GOPATH:=$(shell go env GOPATH)
.PHONY: init
init:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install github.com/micro/micro/v5/cmd/protoc-gen-micro@latest
	go install github.com/micro/micro/v5/cmd/protoc-gen-openapi@latest

.PHONY: api
api:
	protoc --openapi_out=. --proto_path=. proto/users.proto

.PHONY: proto
proto:
	protoc --proto_path=. --micro_out=. --go_out=:. proto/users.proto
	
.PHONY: build
build:
	go build -o users *.go

.PHONY: test
test:
	go test -v ./... -cover

.PHONY: docker
docker:
	docker build . -t users:latest


.PHONY: ent new-entity
ent:
	@echo "Generating Entgo client code ..."
	go generate ./ent 

new-entity:
	@echo "Creating new Entgo entity..."
	go run -mod=mod entgo.io/ent/cmd/ent new $(name)