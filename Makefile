.PHONY: build-docker

# Current  version
VERSION ?= latest
# Default image tag
RECEIVER_IMG ?= quay.io/aneeshkp/cloudevents-receiver:$(VERSION)
SENDER_IMG ?= quay.io/aneeshkp/cloudevents-sender:$(VERSION)
SIDECAR_IMG ?= quay.io/aneeshkp/sidecar-yolo:$(VERSION)
CNF_IMG ?= quay.io/aneeshkp/cnf-yolo:$(VERSION)

# Export GO111MODULE=on to enable project to be built from within GOPATH/src
export GO111MODULE=on
export CGO_ENABLED=0

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

run-amqp-consumer:
	go run ./cmd/amqp/consumer/main.go --config ./config/amqp/config.yml

#run-amqp-sender:
#	go run ./cmd/amqp/producer/main.go --config ./config/amqp/config.yml

run-sidecar:
	go run ./cmd/sidecar/main.go

run-cnf:
	go run ./cmd/cnf/main.go

clean:
	rm ./bin/amqpReceiver
	rm ./bin/amqpSender
	rm ./bin/httpReceiver
	rm ./bin/httpSender
	rm ./bin/*

docker-build: build
	#docker build -f receiver-Dockerfile -t $(RECEIVER_IMG) .
	#docker build -f sender-Dockerfile -t $(SENDER_IMG) .
	#docker build -f yolo-cnf-Dockerfile -t $(CNF_IMG) .
	#docker build -f sidecar-Dockerfile -t $(SIDECAR_IMG) .

	docker build -f DockerFile/receiver -t $(RECEIVER_IMG) .
	docker build -f DockerFile/sender -t $(SENDER_IMG) .
	docker build -f DockerFile/cnf -t $(CNF_IMG) .
	docker build -f DockerFile/sidecar -t $(SIDECAR_IMG) .




# Push the docker image
docker-push:
	docker push ${RECEIVER_IMG}
	docker push ${SENDER_IMG}
	docker push ${CNF_IMG}
	docker push ${SIDECAR_IMG}

build:
	go fmt ./...
	make lint
	#CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/amqpSender ./cmd/amqp/producer/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/amqpReceiver ./cmd/amqp/consumer/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/httpSender ./cmd/http/send/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/httpReceiver ./cmd/http/receive/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/cnf-yolo ./cmd/cnf/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/sidecar-yolo ./cmd/sidecar/main.go
	cp ./config/amqp/config.yml ./bin/config.yml

# Deploy all in the configured Kubernetes cluster in ~/.kube/config
deploy: kustomize
    # && $(KUSTOMIZE) edit set image producer=${SENDER_IMG}
	cd ./manifests && $(KUSTOMIZE) edit set image consumer=${RECEIVER_IMG}
	cd ./manifests && $(KUSTOMIZE) edit set image cnf=${CNF_IMG} && $(KUSTOMIZE) edit set image sidecar=${SIDECAR_IMG}
	$(KUSTOMIZE) build ./manifests | kubectl apply -f -

deploy-qdr: kustomize
	$(KUSTOMIZE) build ./manifests/qpid-router | kubectl apply -f -

deploy-remote: kustomize
	$(KUSTOMIZE) build ./manifests/remote-listener | kubectl apply -f -

# Uninstall from a cluster
uninstall: kustomize
	$(KUSTOMIZE) build ./manifests | kubectl delete -f -

# Uninstall from a cluster
uninstall-qdr: kustomize
	$(KUSTOMIZE) build ./manifests/qpid-router | kubectl delete -f -

# Uninstall from a cluster
uninstall-remote: kustomize
	$(KUSTOMIZE) build ./manifests/remote-listener | kubectl delete -f -

# Uninstall from a cluster
uninstall-all: kustomize
	$(KUSTOMIZE) build ./manifests | kubectl delete -f -
	$(KUSTOMIZE) build ./manifests/remote-listener | kubectl delete -f -
	$(KUSTOMIZE) build ./manifests/qpid-router | kubectl delete -f -

#Install kube-dns addon
kube-dns: kustomize
	$(KUSTOMIZE) build ./kube-dns | kubectl apply -f -

lint:
	golint `go list ./... | grep -v vendor`
	golangci-lint run


