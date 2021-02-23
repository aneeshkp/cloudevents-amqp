package chain

import (
	"fmt"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
)

// State represents the transition states
type State struct {
	ID int
	Payload types.Subscription
	Probability int
}

// DoAction will perform an action for the current state
func (s *State) DoAction() {

	// TODO: add some json action here

	fmt.Printf("TODO: payload printing, payload %v \n",  s.Payload)

}
