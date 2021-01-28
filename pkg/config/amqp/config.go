package amqp

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"gopkg.in/yaml.v2"
)

const (
	configCLIKey = "config"
	configName   = "config.yml"
	filePerm     = 0644
)

var (
	configPath = flag.String(configCLIKey, configName, "path to config file")
)

// Config main configuration for amqp
type Config struct {
	MsgCount int `yaml:"msgcount" json:"msgcount"`
	HostName string   `yaml:"hostname" json:"hostname"`
	Port     int      `yaml:"port" json:"port"`
	Listener Listener `yaml:"listener" json:"listener"`
	Sender   Sender   `yaml:"sender" json:"sender"`
}

// Listener config
type Listener struct {
	Count int     `yaml:"count" json:"count"`
	Queue []Queue `yaml:"queues" json:"queues"`
}

// Sender config
type Sender struct {
	Count int     `yaml:"count" json:"count"`
	Queue []Queue `yaml:"queues" json:"queues"`
}

// Queue config
type Queue struct {
	Name  string `yaml:"name" json:"name"`
	Count int    `yaml:"count" json:"count"`
}

// NewConfig  returns a new decoded AMQPConfig struct
func NewConfig(configPath string) (*Config, error) {
	var file *os.File
	var err error
	// Create config structure
	config := &Config{}
	// Open config file
	if file, err = os.Open(configPath); err != nil {
		return nil, err
	}
	defer file.Close()
	// Init new YAML decode
	d := yaml.NewDecoder(file)
	// Start YAML decoding from file
	if err := d.Decode(&config); err != nil {
		return nil, err
	}
	return config, nil
}

// ValidateConfigPath just makes sure, that the path provided is a file,
// that can be read
func ValidateConfigPath(path string) error {
	s, err := os.Stat(path)
	if err != nil {
		return err
	}
	if s.IsDir() {
		return fmt.Errorf("'%s' is a directory, not a normal file", path)
	}
	return nil
}

// parseFlags will create and parse the CLI flags
// and return the path to be used elsewhere
func parseFlags() (string, error) {
	flag.Parse()
	// Validate the path first
	if err := ValidateConfigPath(*configPath); err != nil {
		return "", err
	}
	// Return the configuration path
	return *configPath, nil
}

// GetConfig returns the AMQPConfig configuration.
func GetConfig() (*Config, error) {
	// Generate our config based on the config supplied
	// by the user in the flags
	cfgPath, err := parseFlags()
	if err != nil {
		return nil, err
	}
	cfg, err := NewConfig(cfgPath)
	return cfg, err
}

// SaveConfig writes configuration to a file at the given config path
func (c *Config) SaveConfig(configPath string) (err error) {
	bytes, _ := yaml.Marshal(c)
	if err != nil {
		return
	}
	err = ioutil.WriteFile(configPath, bytes, filePerm)
	return
}
