// Package pflog defines all of the pflog package
package pflog

import "encoding/json"

var jsonformatterTypeID = "json"

type JSONFormatter struct {
	timeFormat  string
	prettyPrint bool
}

type JSONOutputFormat struct {
	TimeStamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Tags      map[string]interface{} `json:"tags"`
}

// ID returns the specified ID of this formatter
func (jf *JSONFormatter) ID() string {
	return jsonformatterTypeID
}

// SetTimestampFormat sets the time stamp format
// to the provided string representation for outputing
// time/date information
func (jf *JSONFormatter) SetTimestampFormat(format string) {
	jf.timeFormat = format
}

// SetPrettyPrint indicates that the output should be formatted with
// proper indenting opposed to one long string
func (jf *JSONFormatter) SetPrettyPrint(prettyPrint bool) {
	jf.prettyPrint = prettyPrint
}

// Format formats a log entry into a json
// entry such as:
// "message": "whatever"
// "level": "information"
// etc
func (jf *JSONFormatter) Format(entry *Entry) []byte {
	var err error

	jsonOutput := JSONOutputFormat{}
	jsonOutput.TimeStamp = entry.timestamp.Local().Format(jf.timeFormat)
	jsonOutput.Level, err = convertLevelToString(entry.level, true)
	if err != nil {
		jsonOutput.Level = err.Error()
	}
	for _, v := range entry.tags {
		jsonOutput.Tags[v.name] = v.value
	}
	jsonOutput.Message = entry.message

	var formattedMessage []byte
	var marshallErr error

	if jf.prettyPrint {
		formattedMessage, marshallErr = json.MarshalIndent(jsonOutput, "", "	")
	} else {
		formattedMessage, marshallErr = json.Marshal(jsonOutput)
	}

	if marshallErr != nil {
		formattedMessage = []byte(marshallErr.Error())
	}
	return formattedMessage
}
