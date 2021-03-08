.PHONY: build-docker

# Current  version
VERSION ?=1.0
# Default image tag

SIDECAR_IMG ?= quay.io/aneeshkp/sidecar:$(VERSION)
PTP_SIDECAR_IMG ?= quay.io/aneeshkp/ptpsidecar:$(VERSION)
PTP_IMG ?= quay.io/aneeshkp/ptp:$(VERSION)
VDU_IMG ?= quay.io/aneeshkp/vdu:$(VERSION)

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

run-vdu-sidecar:
	go run ./cmd/sidecar/main.go --config ./pkg/config/vdu/config.yml

run-vdu:
	go run ./cmd/vdu/main.go --config ./pkg/config/vdu/config.yml

run-ptp-sidecar:
	go run ./cmd/sidecar/main.go --config ./pkg/config/ptp/config.yml

run-ptp:
	go run ./cmd/ptp/main.go --config ./pkg/config/ptp/config.yml

clean:
	rm -rf ./bin/*

docker-build: build
	docker build -f DockerFile/vdu -t $(VDU_IMG) .
	docker build -f DockerFile/sidecar -t $(SIDECAR_IMG) .
	docker build -f DockerFile/ptp-sidecar -t $(PTP_SIDECAR_IMG) .
	docker build -f DockerFile/ptp -t $(PTP_IMG) .

# Push the docker image
docker-push:
	docker push ${PTP_IMG}
	docker push ${VDU_IMG}
	docker push ${SIDECAR_IMG}
	docker push ${PTP_SIDECAR_IMG}

build:
	go fmt ./...
	make lint
	mkdir -p ./bin/config/ptp
	mkdir -p ./bin/config/vdu
	cp ./pkg/config/ptp/config.yml ./bin/config/ptp/config.yml
	cp ./pkg/config/vdu/config.yml ./bin/config/vdu/config.yml
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/ptp ./cmd/ptp/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/vdu ./cmd/vdu/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/sidecar ./cmd/sidecar/main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/ptpsidecar ./cmd/sidecar/main.go


# Deploy all in the configured Kubernetes cluster in ~/.kube/config
deploy: kustomize
    # && $(KUSTOMIZE) edit set image producer=${SENDER_IMG}
	cd ./manifests && $(KUSTOMIZE) edit set image ptp=${PTP_IMG} && $(KUSTOMIZE) edit set image sidecar=${SIDECAR_IMG} && $(KUSTOMIZE) edit set image ptpsidecar=${PTP_SIDECAR_IMG} && $(KUSTOMIZE) edit set image vdu=${VDU_IMG}
	$(KUSTOMIZE) build ./manifests | kubectl apply -f -

deploy-qdr: kustomize
	$(KUSTOMIZE) build ./manifests/qpid-router | kubectl apply -f -


# Uninstall from a cluster
uninstall: kustomize
	$(KUSTOMIZE) build ./manifests | kubectl delete -f -

# Uninstall from a cluster
uninstall-qdr: kustomize
	$(KUSTOMIZE) build ./manifests/qpid-router | kubectl delete -f -

# Uninstall from a cluster
uninstall-other: kustomize
	$(KUSTOMIZE) build ./manifests/manifests | kubectl delete -f -

# Uninstall from a cluster
uninstall-all: kustomize
	$(KUSTOMIZE) build ./manifests | kubectl delete -f -
	$(KUSTOMIZE) build ./manifests/qpid-router | kubectl delete -f -

#Install kube-dns addon
kube-dns: kustomize
	$(KUSTOMIZE) build ./kube-dns | kubectl apply -f -

lint:
	golint `go list ./... | grep -v vendor`
	golangci-lint run


