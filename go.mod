module github.com/aneeshkp/cloudevents-amqp

go 1.14

require (
	github.com/Azure/go-amqp v0.12.7
	github.com/cloudevents/sdk-go/protocol/amqp/v2 v2.3.1
	github.com/cloudevents/sdk-go/v2 v2.3.1
	github.com/fsnotify/fsnotify v1.4.9
	github.com/go-fsnotify/fsnotify v0.0.0-20180321022601-755488143dae // indirect

	//github.com/aneeshkp/sdk-go/protocol/amqp/v2 v2.3.1
	//github.com/aneeshkp/sdk-go/v2 v2.3.1

	github.com/google/uuid v1.1.1
	github.com/gorilla/mux v1.8.0
	github.com/jmcvetta/randutil v0.0.0-20150817122601-2bb1b664bcff
	github.com/stretchr/testify v1.5.1
	golang.org/x/sys v0.0.0-20210225134936-a50acf3fe073 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	gopkg.in/yaml.v2 v2.3.0
)

replace (
	github.com/cloudevents/sdk-go/protocol/amqp/v2 => /home/aputtur/gocode/src/github.com/aneeshkp/sdk-go/protocol/amqp/v2
	github.com/cloudevents/sdk-go/v2 => /home/aputtur/gocode/src/github.com/aneeshkp/sdk-go/v2

)
