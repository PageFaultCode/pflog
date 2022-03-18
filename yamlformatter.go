// Package pflog defines all of the pflog package
package pflog

import (
	"gopkg.in/yaml.v3"
)

var yamlformatterTypeID = "yaml"

type YAMLFormatter struct {
	timeFormat string
}

type YAMLOutputFormat struct {
	TimeStamp string                 `yaml:"timestamp"`
	Level     string                 `yaml:"level"`
	Message   string                 `yaml:"message"`
	Tags      map[string]interface{} `yaml:"tags"`
}

// ID returns the specified ID of this formatter
func (yf *YAMLFormatter) ID() string {
	return yamlformatterTypeID
}

// SetTimestampFormat sets the time stamp format
// to the provided string representation for outputing
// time/date information
func (yf *YAMLFormatter) SetTimestampFormat(format string) {
	yf.timeFormat = format
}

// Format formats a log entry into a json
// entry such as:
// "message": "whatever"
// "level": "information"
// etc
func (yf *YAMLFormatter) Format(entry *Entry) []byte {
	var err error

	yamlOutput := YAMLOutputFormat{}
	yamlOutput.TimeStamp = entry.timestamp.Local().Format(yf.timeFormat)
	yamlOutput.Level, err = convertLevelToString(entry.level, true)
	if err != nil {
		yamlOutput.Level = err.Error()
	}
	for _, v := range entry.tags {
		yamlOutput.Tags[v.name] = v.value
	}
	yamlOutput.Message = entry.message

	formattedMessage, marshallErr := yaml.Marshal(yamlOutput)

	if marshallErr != nil {
		formattedMessage = []byte(marshallErr.Error())
	}
	return formattedMessage
}
