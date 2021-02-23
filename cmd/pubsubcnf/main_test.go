package main

import (
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestEvent(t *testing.T) {
	payload := types.Subscription{}
	err := httpEvent(payload,"1212")
	assert.Nil(t, err)
}
