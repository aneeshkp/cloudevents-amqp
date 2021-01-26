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

kustomize:
ifeq (, $(shell which kustomize))
	@{ \
	set -e ;\
	KUSTOMIZE_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$KUSTOMIZE_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/kustomize/kustomize/v3@v3.5.4 ;\
	rm -rf $$KUSTOMIZE_GEN_TMP_DIR ;\
	}
KUSTOMIZE=$(GOBIN)/kustomize
else
KUSTOMIZE=$(shell which kustomize)
endif

build:
	go fmt ./...
	make lint
	go build -o ./bin/amqpSender ./cmd/amqp/producer/main.go
	go build -o ./bin/amqpReceiver ./cmd/amqp/consumer/main.go
	go build -o ./bin/httpSender ./cmd/http/send/main.go
	go build -o ./bin/httpReceiver ./cmd/http/receive/main.go


clean:
	rm ./bin/amqpReceiver
	rm ./bin/amqpSender
	rm ./bin/httpReceiver
	rm ./bin/httpSender

docker-build:
	docker build -f receiver-Dockerfile -t $(RECEIVER_IMG) .
	docker build -f sender-Dockerfile -t $(SENDER_IMG) .

# Push the docker image
docker-push:
	docker push ${RECEIVER_IMG} && docker push ${SENDER_IMG}

# Deploy all in the configured Kubernetes cluster in ~/.kube/config
deploy: kustomize
	cd ./manifests && $(KUSTOMIZE) edit set image consumer=${RECEIVER_IMG} && $(KUSTOMIZE) edit set image producer=${SENDER_IMG}
	$(KUSTOMIZE) build ./manifests | kubectl apply -f -

# Uninstall from a cluster
uninstall: kustomize
	$(KUSTOMIZE) build ./manifests | kubectl delete -f -

#Install kube-dns addon
kube-dns: kustomize
	$(KUSTOMIZE) build ./kube-dns | kubectl apply -f -

lint:
	golint `go list ./... | grep -v vendor`
	golangci-lint run


