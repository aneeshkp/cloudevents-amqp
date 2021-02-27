package config

import (
	"flag"
	"fmt"
	"github.com/aneeshkp/cloudevents-amqp/pkg/types"
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

//StatusResource name the status you want to check
type StatusResource struct {
	Name string `yaml:"name" json:"name"`
}

//HostConfig , configurations for containers
type HostConfig struct {
	HostName string `yaml:"hostname" json:"hostname"`
	Port     int    `yaml:"port" json:"port"`
}

//SocketConfig , configurations for sockets
type SocketConfig struct {
	Listener HostConfig `yaml:"listener" json:"listener"`
	Sender   HostConfig `yaml:"ptp" json:"ptp"`
}

//Cluster , configurations for cluster info
type Cluster struct {
	Name      string `yaml:"name" json:"name"`
	Node      string `yaml:"node" json:"node"`
	NameSpace string `yaml:"namespace" json:"namespace"`
}

// Config main configuration for amqp
type Config struct {
	AMQP           HostConfig         `yaml:"amqp" json:"amqp"`
	API            HostConfig         `yaml:"api" json:"api"`
	Host           HostConfig         `yaml:"host" json:"host"`
	Socket         SocketConfig       `yaml:"socket" json:"socket"`
	Cluster        Cluster            `yaml:"cluster" json:"cluster"`
	StatusResource []StatusResource   `yaml:"statusresource" json:"statusresource"`
	PubFilePath    string             `json:"pubfilepath,omitempty,string"` //pub.json
	SubFilePath    string             `json:"subfilepath,omitempty,string"` //sub.json
	PublishStatus  bool               `json:"publishstatus,omitempty,bool"` //nolint:staticcheck
	EventHandler   types.EventHandler `yaml:"eventhandler" json:"eventandler"`
	APIPathPrefix  string             `yaml:"apipathprefix" json:"apipathprefix"`
	HostPathPrefix string             `yaml:"hostpathprefix" json:"hostpathprefix"`
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

//DefaultConfig fills up teh default configurations
func DefaultConfig(defaultHosPort, defaultAPIPort, senderSocketPort, listenerSocketPort int, cluster, node, namespace string, publishStatus bool) *Config {
	if cluster == "" {
		cluster = "clusternameunknown"
	}
	if node == "" {
		node = "nodenameunknown"
	}
	if namespace == "" {
		namespace = "namespaceunknown"
	}

	cfg := &Config{
		AMQP: HostConfig{
			HostName: "amqp://localhost",
			Port:     5672,
		},
		API: HostConfig{
			HostName: "localhost",
			Port:     defaultHosPort,
		},
		Host: HostConfig{
			HostName: "localhost",
			Port:     defaultAPIPort,
		},
		Socket: SocketConfig{
			Listener: HostConfig{
				HostName: "localhost",
				Port:     listenerSocketPort,
			},
			Sender: HostConfig{
				HostName: "localhost",
				Port:     senderSocketPort,
			},
		},
		Cluster: Cluster{
			Name:      cluster,
			Node:      node,
			NameSpace: namespace,
		}, StatusResource: []StatusResource{{Name: "PTP"}},
		PubFilePath:   "pub.json",
		SubFilePath:   "sub.json",
		PublishStatus: publishStatus,
	}
	return cfg
}
