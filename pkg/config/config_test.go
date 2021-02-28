package config_test

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"

	eventConfig "github.com/aneeshkp/cloudevents-amqp/pkg/config"
	"github.com/stretchr/testify/assert"
)

var (
	file       *os.File
	err        error
	testCfg    *eventConfig.Config
	configPath *string
)

func loadConfig() {

	testCfg := eventConfig.DefaultConfig(8080, 9090, 2001, 2002, "unknown", "unknonw", "unknown")

	var name []string
	name = append(name, "test1")
	testCfg.StatusResource.Name = name
	testCfg.StatusResource.Status.PublishStatus = false
	testCfg.StatusResource.Status.EnableStatusCheck = false

}

func setup() {
	file, err = ioutil.TempFile(".", "config.yml")
	if err != nil {
		log.Fatal(err)
	}
	testCfg = &eventConfig.Config{}
	loadConfig()
	err = testCfg.SaveConfig(file.Name())
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
	cfg, err := eventConfig.NewConfig(file.Name())
	assert.NotNil(t, cfg)
	assert.Equal(t, cfg.Host.HostName, "localhost")
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
			err := eventConfig.ValidateConfigPath(tt.path)
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
		conf  *eventConfig.Config
		error string
	}{
		{[]string{"./ptp"}, testCfg, "no such file or directory"},
		{[]string{"./ptp", "-config", "config_not_exists"}, testCfg, "no such file or directory"},
		{[]string{"./ptp", "-config", path}, testCfg, "is a directory, not a normal file"},
		{[]string{"./ptp", "-config", file.Name()}, testCfg, ""},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			tt := tt // pin
			os.Args = tt.args
			if len(tt.args) > 2 {
				configPath = &tt.args[2]
			}
			testConfig, err := eventConfig.GetConfig()
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
