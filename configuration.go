// Package pflog defines all of the pflog package
package pflog

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Settings struct {
	Level        string `yaml:"level"`
	TriggerLevel string `yaml:"trigger_level"`
	Backlog      int    `yaml:"backlog"`
}

type FormatterEntry struct {
	ID              string `yaml:"id"`
	Filename        string `yaml:"filename,omitempty"`
	TimestampFormat string `yaml:"timestamp_format,omitempty"` // Go time layout; defaults to RFC3339
}

type Configuration struct {
	Settings   Settings         `yaml:"settings"`
	Formatters []FormatterEntry `yaml:"formatters"`
	UserLog    *Log             // will be non-nil if specified
}

func (configuration *Configuration) LoadConfigurationFile(filename string) error {
	configuration.UserLog = nil
	fileContents, err := ioutil.ReadFile(filepath.Clean(filename))

	if err != nil {
		return err
	}

	err = yaml.Unmarshal(fileContents, configuration)
	if err != nil {
		return err
	}

	return configuration.LoadConfiguration()
}

func (configuration *Configuration) LoadConfiguration() error {
	log := New()
	err := log.SetLevel(convertStringToLevel(configuration.Settings.Level))
	if err != nil {
		return err
	}
	err = log.SetTriggerLevel(convertStringToLevel(configuration.Settings.TriggerLevel))
	if err != nil {
		return err
	}
	err = log.SetBacklogDepth(configuration.Settings.Backlog)
	if err != nil {
		return err
	}

	for _, v := range configuration.Formatters {
		formatter, createErr := CreateFormatter(v.ID)
		if createErr != nil {
			continue
		}
		tsFormat := v.TimestampFormat
		if tsFormat == "" {
			tsFormat = time.RFC3339
		}
		formatter.SetTimestampFormat(tsFormat)
		outFile, fileErr := os.Create(filepath.Clean(v.Filename))
		if fileErr != nil {
			continue
		}
		_ = log.AddOutputTargetAndFormatter(outFile, formatter)
	}

	configuration.UserLog = log

	return nil
}

func (configuration *Configuration) GetLogger() *Log {
	return configuration.UserLog
}
