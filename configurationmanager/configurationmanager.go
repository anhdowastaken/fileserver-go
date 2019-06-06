package configurationmanager

import (
	"fmt"
	"strings"
	"sync"

	"github.com/spf13/viper"

	"github.com/anhdowastaken/fileserver-go/logger"
)

// AppConfig structure contains main configuration of the app
type AppConfig struct {
	FilelogDestination string `mapstructure:"filelog_destination"`
	LogEnable          bool   `mapstructure:"log_enable"`
	LogLevel           int    `mapstructure:"log_level"`
	LogRotationTime    int    `mapstructure:"log_rotation_time"`
	MaxLogSize         int    `mapstructure:"max_log_size"`
}

type HTTPConfig struct {
	Address             string        `mapstructure:"address"`
	SSL                 bool          `mapstructure:"ssl"`
	KeyFile             string        `mapstructure:"key_file"`
	CertFile            string        `mapstructure:"cert_file"`
	MaxFileSize         int           `mapstructure:"max_file_size"`
	FileServerDirectory string        `mapstructure:"file_server_directory"`
	Authen              []BasicAuthen `mapstructure:"basic_authen"`
}

type BasicAuthen struct {
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// ConfigurationManager structure
type ConfigurationManager struct {
	mutex      sync.Mutex
	appConfig  AppConfig
	httpConfig HTTPConfig
	v          *viper.Viper
}

var instance *ConfigurationManager
var once sync.Once

// New function initialize singleton ConfigurationManager
func New() *ConfigurationManager {
	once.Do(func() {
		instance = &ConfigurationManager{}
		instance.v = viper.New()
	})

	return instance
}

// Load function loads and validates configuration from input file
func (cm *ConfigurationManager) Load(configurationFile string) error {
	mlog := logger.New()

	var tmp ConfigurationManager
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	cm.v.SetConfigFile(configurationFile)
	cm.v.SetConfigType("toml")
	err := cm.v.ReadInConfig()
	if err != nil {
		return err
	}

	// Load application config
	err = cm.v.UnmarshalKey("app", &tmp.appConfig)
	if err != nil {
		return fmt.Errorf("[app] part of config file is not valid: %s \n", err)
	}

	mi := cm.v.Get("app")
	if mi == nil {
		return fmt.Errorf("[app] part of config file is not valid\n")
	}

	m := mi.(map[string]interface{})
	if m["log_enable"] == nil {
		tmp.appConfig.LogEnable = true
	}

	if m["log_level"] == nil {
		tmp.appConfig.LogLevel = logger.INFO // By default, log level is INFO
	} else {
		logLevel, ok := m["log_level"].(int64)
		if !ok || (logLevel < logger.FATAL || logLevel > logger.DEBUG) {
			tmp.appConfig.LogLevel = logger.INFO
		}
	}

	if m["log_rotation_time"] == nil {
		tmp.appConfig.LogRotationTime = 60 // By default, log will be rotated after 60 minutes
	} else {
		logRotationTime, ok := m["log_rotation_time"].(int64)
		if !ok || logRotationTime <= 0 {
			tmp.appConfig.LogRotationTime = 60
		}
	}

	if m["max_log_size"] == nil {
		tmp.appConfig.MaxLogSize = 500 // By default, maximum size of each log file is 500MB
	} else {
		maxLogSize, ok := m["max_log_size"].(int64)
		if !ok || maxLogSize <= 0 {
			tmp.appConfig.MaxLogSize = 500
		}
	}

	err = cm.v.UnmarshalKey("http", &tmp.httpConfig)
	if err != nil {
		return fmt.Errorf("[http] part of config file is not valid: %s \n", err)
	}

	mi = cm.v.Get("http")
	if mi == nil {
		return fmt.Errorf("[http] part of config file is not valid\n")
	}

	m = mi.(map[string]interface{})
	if m["address"] == nil || strings.TrimSpace(m["address"].(string)) == "" {
		tmp.httpConfig.Address = ":9000"
	}

	if m["ssl"] == nil {
		tmp.httpConfig.SSL = false
	}

	if m["max_file_size"] == nil {
		tmp.httpConfig.MaxFileSize = 10
	} else {
		maxFileSize, ok := m["max_file_size"].(int64)
		if !ok || maxFileSize <= 0 {
			tmp.httpConfig.MaxFileSize = 10
		}
	}

	if m["file_server_directory"] == nil || strings.TrimSpace(m["file_server_directory"].(string)) == "" {
		return fmt.Errorf("file server directory is empty")
	}

	cm.appConfig = tmp.appConfig

	cm.appConfig.FilelogDestination = strings.TrimSpace(cm.appConfig.FilelogDestination)

	cm.httpConfig = tmp.httpConfig
	cm.httpConfig.Address = strings.TrimSpace(cm.httpConfig.Address)
	cm.httpConfig.FileServerDirectory = strings.TrimSpace(cm.httpConfig.FileServerDirectory)

	mlog.SetLevel(cm.appConfig.LogLevel)

	return nil
}

// GetAppConfig returns configuration of the app
func (cm ConfigurationManager) GetAppConfig() AppConfig {
	return cm.appConfig
}

// GetHTTPConfig returns configuration of the HTTP server
func (cm ConfigurationManager) GetHTTPConfig() HTTPConfig {
	return cm.httpConfig
}
