```
Running on Minikube
 Required to have kube-dns configured 
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

