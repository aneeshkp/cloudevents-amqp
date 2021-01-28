package types

import (
	"context"
	amqp1 "github.com/cloudevents/sdk-go/protocol/amqp/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
)

// AMQPProtocol loads clients
type AMQPProtocol struct {
	ID               string
	MsgCount         int
	Protocol         *amqp1.Protocol
	Ctx              context.Context
	ParentContext    context.Context
	CancelFn         context.CancelFunc
	Client           cloudevents.Client
	Queue            string
	MaxDiff          int64
	MsgReceivedCount uint64
}
