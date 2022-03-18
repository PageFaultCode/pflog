// Package pflog defines all of the pflog package
package pflog

import "time"

type Entry struct {
	level     LogLevel
	timestamp time.Time
	message   string
	tags      []*Tag
}

func NewEntry(level LogLevel, timestamp time.Time, message string, tagset []*Tag) *Entry {
	return &Entry{
		level:     level,
		timestamp: timestamp,
		message:   message,
		tags:      tagset,
	}
}
