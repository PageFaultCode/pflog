// Package pflog defines all of the pflog package
package pflog

import "fmt"

var formatterTypeID = "text"

type TextFormatter struct {
	timeFormat string
}

func (tf *TextFormatter) ID() string {
	return formatterTypeID
}

func (tf *TextFormatter) SetTimestampFormat(format string) {
	tf.timeFormat = format
}

func (tf *TextFormatter) Format(entry *Entry) []byte {
	dateString := entry.timestamp.Local().Format(tf.timeFormat)
	levelString, err := convertLevelToString(entry.level, true)
	if err != nil {
		levelString = err.Error()
	}
	tagString := ""
	for _, v := range entry.tags {
		tagString += v.name + ": " + fmt.Sprintf("%v", v.value) + " "
	}

	formattedMessage := dateString + " [" + levelString + "] " + tagString + entry.message + "\n"

	return []byte(formattedMessage)
}
