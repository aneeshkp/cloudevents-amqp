module github.com/aneeshkp/cloudevents-amqp

go 1.14

require (
	github.com/Azure/go-amqp v0.12.7
	github.com/cloudevents/sdk-go/protocol/amqp/v2 v2.3.1
	github.com/cloudevents/sdk-go/v2 v2.3.1

	//github.com/aneeshkp/sdk-go/protocol/amqp/v2 v2.3.1
	//github.com/aneeshkp/sdk-go/v2 v2.3.1

	github.com/google/uuid v1.1.1
	github.com/stretchr/testify v1.5.1
	gopkg.in/yaml.v2 v2.3.0
)

replace (
	github.com/cloudevents/sdk-go/protocol/amqp/v2 => /home/aputtur/gocode/src/github.com/aneeshkp/sdk-go/protocol/amqp/v2
	github.com/cloudevents/sdk-go/v2 => /home/aputtur/gocode/src/github.com/aneeshkp/sdk-go/v2

)
