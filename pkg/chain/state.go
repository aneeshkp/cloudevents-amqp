package chain

import (
	"fmt"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
)

// State represents the transition states
type State struct {
	Payload types.Message
	ID      int
}

// DoAction will perform an action for the current state
func (s *State) DoAction() {

	// TODO: add some json action here

	fmt.Printf("TODO: payload printing, id %d, payload %v \n", s.ID, s.Payload)

}
