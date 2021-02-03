package main

import (
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestEvent(t *testing.T) {
	payload := types.Message{}
	err := Event(payload)
	assert.Nil(t, err)
}
