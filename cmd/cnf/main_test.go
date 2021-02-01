package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestEvent(t *testing.T) {
	err := Event()
	assert.Nil(t, err)
}
