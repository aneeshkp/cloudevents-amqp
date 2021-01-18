.PHONY: build-docker

# Current  version
VERSION ?= latest
# Default image tag
RECEIVER_IMG ?= quay.io/aneeshkp/cloudevents-receiver:$(VERSION)
SENDER_IMG ?= quay.io/aneeshkp/cloudevents-sender:$(VERSION)

# Export GO111MODULE=on to enable project to be built from within GOPATH/src
export GO111MODULE=on

ifeq (,$(shell go env GOBIN))
  GOBIN=$(shell go env GOPATH)/bin
else
  GOBIN=$(shell go env GOBIN)
endif

export COMMON_GO_ARGS=-race

build:
	go fmt ./...
	make lint

docker-build:
	docker build -f receiver-Dockerfile -t $(RECEIVER_IMG) .
	docker build -f sender-Dockerfile -t $(SENDER_IMG) .

# Push the docker image
docker-push:
	docker push ${RECEIVER_IMG} && docker push ${SENDER_IMG}

