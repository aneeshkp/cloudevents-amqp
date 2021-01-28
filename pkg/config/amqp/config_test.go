package amqp_test

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"

	amqpConfig "github.com/aneeshkp/cloudevents-amqp/pkg/config/amqp"
	"github.com/stretchr/testify/assert"
)

var (
	file       *os.File
	err        error
	test       amqpConfig.Config
	configPath *string
)

const (
	queueName1 = "test"
	queueName2 = "test"
	queueCount = 10
)

func loadConfig() {

	test.Sender = amqpConfig.Sender{
		Count: queueCount,
		Queue: []amqpConfig.Queue{
			{
				Name:  queueName1,
				Count: queueCount,
			},
			{
				Name:  queueName2,
				Count: queueCount,
			},
		},
	}

	test.Listener = amqpConfig.Listener{
		Count: queueCount,
		Queue: []amqpConfig.Queue{
			{
				Name:  queueName1,
				Count: queueCount,
			},
			{
				Name:  queueName2,
				Count: queueCount,
			},
		},
	}
	test.HostName = "amqps://localhost"
	test.Port = 5672
}

func setup() {
	file, err = ioutil.TempFile(".", "config.yml")
	if err != nil {
		log.Fatal(err)
	}
	test = amqpConfig.Config{}
	loadConfig()
	err = test.SaveConfig(file.Name())
	if err != nil {
		log.Fatal(err)
	}
}

func teardown() {
	if file != nil {
		os.Remove(file.Name())
	}

}

func TestFullConfigLoad(t *testing.T) {
	setup()
	defer (teardown)()
	cfg, err := amqpConfig.NewConfig(file.Name())
	assert.NotNil(t, cfg)
	assert.Equal(t, len(cfg.Listener.Queue), 2)
	assert.Equal(t, cfg.Listener.Queue[0].Name, queueName1)
	assert.Nil(t, err)
}

func TestValidateConfigPath(t *testing.T) {
	var tests = []struct {
		path  string
		error error
	}{
		{".", fmt.Errorf("'.' is a directory, not a normal file")},
		{"./config", fmt.Errorf("./config is a directory, not a normal file")},
		{"./config.yml", nil},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			tt := tt // pin
			err := amqpConfig.ValidateConfigPath(tt.path)
			if err == nil && tt.error != nil {
				assert.Fail(t, err.Error())
			}
		})
	}
}

// TestConfigLoadFunction ... Test if config is read correctly
func TestConfigLoadFunction(t *testing.T) {
	setup()
	defer (teardown)()
	path, _ := os.Getwd()
	var tests = []struct {
		args  []string
		conf  amqpConfig.Config
		error string
	}{
		{[]string{"./sender"}, test, "no such file or directory"},
		{[]string{"./sender", "-config", "config_not_exists"}, test, "no such file or directory"},
		{[]string{"./sender", "-config", path}, test, "is a directory, not a normal file"},
		{[]string{"./sender", "-config", file.Name()}, test, ""},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			tt := tt // pin
			os.Args = tt.args
			if len(tt.args) > 2 {
				configPath = &tt.args[2]
			}
			testConfig, err := amqpConfig.GetConfig()
			if err == nil {
				assert.Nil(t, err)
				assert.NotNil(t, testConfig)
				assert.Equal(t, *testConfig, tt.conf)
			} else {
				assert.NotNil(t, err)
				assert.Contains(t, err.Error(), tt.error)
			}
		})
	}
}
