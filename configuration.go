// Package pflog defines all of the pflog package
package pflog

import (
	"errors"
	"fmt"
	"io"
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
	MaxSizeMB       int    `yaml:"max_size_mb,omitempty"`      // rotate when file exceeds this size; 0 = disabled
	MaxBackups      int    `yaml:"max_backups,omitempty"`      // number of rotated files to keep; 0 = keep all
	Compress        bool   `yaml:"compress,omitempty"`         // gzip older backups; newest backup stays plain
}

type Configuration struct {
	Settings   Settings         `yaml:"settings"`
	Formatters []FormatterEntry `yaml:"formatters"`
	UserLog    *Log             // will be non-nil if specified
}

func (configuration *Configuration) LoadConfigurationFile(filename string) error {
	configuration.UserLog = nil
	fileContents, err := os.ReadFile(filepath.Clean(filename))

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
	backlog := configuration.Settings.Backlog
	if backlog < 1 {
		backlog = DefaultBacklogDepth
	}
	err = log.SetBacklogDepth(backlog)
	if err != nil {
		return err
	}

	// configure every valid target; collect failures rather than
	// silently dropping them
	var targetErrs []error
	for _, v := range configuration.Formatters {
		formatter, createErr := CreateFormatter(v.ID)
		if createErr != nil {
			targetErrs = append(targetErrs, createErr)
			continue
		}
		tsFormat := v.TimestampFormat
		if tsFormat == "" {
			tsFormat = time.RFC3339
		}
		formatter.SetTimestampFormat(tsFormat)
		var outWriter io.Writer
		if v.Filename == "stdout" {
			outWriter = os.Stdout
		} else if v.MaxSizeMB > 0 {
			rw, rwErr := newRotatingWriter(filepath.Clean(v.Filename), int64(v.MaxSizeMB)*1024*1024, v.MaxBackups, v.Compress)
			if rwErr != nil {
				targetErrs = append(targetErrs, fmt.Errorf("formatter %s: %w", v.ID, rwErr))
				continue
			}
			outWriter = rw
		} else {
			outFile, fileErr := os.OpenFile(filepath.Clean(v.Filename), os.O_CREATE|os.O_WRONLY|os.O_APPEND, logFileMode)
			if fileErr != nil {
				targetErrs = append(targetErrs, fmt.Errorf("formatter %s: %w", v.ID, fileErr))
				continue
			}
			outWriter = outFile
		}
		_ = log.AddOutputTargetAndFormatter(outWriter, formatter)
	}

	configuration.UserLog = log

	return errors.Join(targetErrs...)
}

func (configuration *Configuration) GetLogger() *Log {
	return configuration.UserLog
}
