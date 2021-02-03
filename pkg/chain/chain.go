package chain

import "github.com/jmcvetta/randutil"

// Chain represents the main markov chain struct
type Chain struct {
	TransitionMatrix [][]float32
	States           []State
}

// Create generates a new Chain struct
func Create(
	transitionMatrix [][]float32,
	states []State,
) Chain {
	return Chain{
		TransitionMatrix: transitionMatrix,
		States:           states,
	}
}

// Action runs the markov action
func (c *Chain) Action(currentStateID int) {
	//  transition probabilities
	currentStateChoice := c.GetStateChoice(currentStateID)
	currentStateID = currentStateChoice.Item.(int)
	currentState := c.GetState(currentStateID)
	currentState.DoAction()

}

// Loop runs the markov loop
func (c *Chain) Loop(start int) {

	// initialize current state
	currentStateID := start
	// loop through the transition probabilities
	for {
		currentStateChoice := c.GetStateChoice(currentStateID)
		currentStateID = currentStateChoice.Item.(int)
		currentState := c.GetState(currentStateID)
		currentState.DoAction()
	}
}

// TransitionProbability returns the probability of transitioning from one state to another
func (c *Chain) TransitionProbability(input, output int) float32 {
	return c.TransitionMatrix[input][output]
}

//GetState ....
func (c *Chain) GetState(id int) State {
	for _, state := range c.States {
		if state.ID == id {
			return state
		}
	}
	return State{}
}

// GetStateChoice ...
func (c *Chain) GetStateChoice(id int) randutil.Choice {

	transitions := c.TransitionMatrix[id-1]
	choices := []randutil.Choice{}
	var weight int

	for i, prob := range transitions {
		weight = int(prob * float32(100))
		choices = append(choices, randutil.Choice{
			Weight: weight,
			Item:   i + 1,
		})
	}

	choice, _ := randutil.WeightedChoice(choices)
	return choice
}
