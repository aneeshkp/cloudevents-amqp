
NOTE: Use Multicast branch .

Install Kustomize
``` 
curl -s "https://raw.githubusercontent.com/\
kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"  | bash

```

```
Running on Minikube
 Required to have kube-dns configured 
```

Dependency :

This project depends on cloudsevents-sdk and current version of cloudevents has an issue , so you need to checkout 
project from this repo. (go mod of clouevents-amqp is modified to use local checkout)
``` 
git clone https://github.com/aneeshkp/sdk-go.git
cd sdk-go
git remote add upstream https://github.com/cloudevents/sdk-go.git
git fetch upstream
git reset --hard upstream/master
git checkout  656-make-separate-receiver-sender-protocol
```


### Description 
This deploys cloud events producer and consumer with AMQP1 protocol binding.
By default, it is set to send 1000 messages which can be overridden in before deploying.
```
manifests/02-producer.yaml
  env:
    - name: AMQP_URL
      value: "amqp://amqp-dispatch-service:5672/test2"
    - name: MSG_COUNT
      value: "1000"
```


## 1. Configuring kube-dns on existing minikube
```
make kube-dns

```

##2. building images
```
set env either in make file or cmd line
# Default image tag is set for deploying without any changes 
#To change set these env  variables 
export RECEIVER_IMG = quay.io/aneeshkp/cloudevents-receiver:latest
export SENDER_IMG = quay.io/aneeshkp/cloudevents-sender:latest

a) make docker-build
b) make  docker-push
(Make sure if you are using quay.io to set the repo as public)
```
##3 Deploying the qdr , consumer and producer 

```
make deploy
```
Tip: Run `make uninstall` if you have already running the pods.

###Post deployment
Make sure all three pods are running 
```
cloud-events-consumer-deployment-56cc94c7c4-mgjsq   1/1     Running     0          6m50s
cloud-events-producer-deployment-7cd567bfd-ptmt6    1/1     Running 
qpid-dispatcherr-deployment-55d6dbc54-sprcg         1/1     Running     0          6m50s0          6m50s
```

Check the logs of the producer pod to see how many were sent and how many failed to settle.

```
kubectl logs -f cloud-events-producer-deployment-7cd567bfd-ptmt6
```

###UnInstall
```
make uninstall

```


##Run AMQP Clinet sender and recevier
```
make run-amqp-consumer
make run-amqp-sender

```