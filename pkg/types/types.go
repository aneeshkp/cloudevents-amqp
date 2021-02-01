package types

import (
	"context"
	amqp1 "github.com/cloudevents/sdk-go/protocol/amqp/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/types"
	"sync/atomic"
	"time"
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
	MinDiff          int64
	MaxDiff2         int64
	MinDiff2         int64
	CurrentMax       int64
	CurrentMax2      int64
	MsgReceivedCount uint64
}

//Result ...
type Result struct {
	ID                    string
	FromSourceMaxDiff     int64
	FromSourceMinDiff     int64
	FromSourceCurrentMax  int64
	FromSideCarMinDiff    int64
	FromSideCarMaxDiff    int64
	FromSideCarCurrentMax int64
	MsgReceivedCount      uint64
}

func (r *Result) Write(l AMQPProtocol) {
	atomic.AddUint64(&r.MsgReceivedCount, 1)
	r.ID = l.ID
	r.FromSourceMaxDiff = l.MaxDiff
	r.FromSourceMinDiff = l.MinDiff
	r.FromSideCarMaxDiff = l.MaxDiff2
	r.FromSideCarMinDiff = l.MinDiff
	r.MsgReceivedCount = l.MsgReceivedCount
	r.FromSideCarCurrentMax = l.CurrentMax2
	r.FromSourceCurrentMax = l.CurrentMax
}

// Message defines the Data of CloudEvent
type Message struct {
	// Msg holds the message from the event
	ID     int              `json:"id,omitempty,string"`
	Source string           `json:"source,omitempty,string"`
	Msg    string           `json:"msg,omitempty,string"`
	Time   *types.Timestamp `json:"time,omitempty"`
}

//GetTime ...
func (m *Message) GetTime() time.Time {
	if m.Time != nil {
		return m.Time.Time
	}
	return time.Time{}
}

// SetTime implements EventContextWriter.SetTime
func (m *Message) SetTime(t time.Time) {
	if t.IsZero() {
		m.Time = nil
	} else {
		m.Time = &types.Timestamp{Time: t}
	}
}
