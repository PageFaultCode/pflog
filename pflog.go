// Package pflog defines all of the pflog package
package pflog

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const (
	Trace = iota
	Debug
	Information
	Warning
	Error
	Fatal
)

var (
	LogLevelTrace       = "trace"
	LogLevelDebug       = "debug"
	LogLevelInformation = "information"
	LogLevelWarning     = "warning"
	LogLevelError       = "error"
	LogLevelFatal       = "fatal"
)

const DefaultBacklogDepth = 500

type LogLevel int

// Log is a type that has both a level at which to log, and a
// trigger level at which to dump all logs leading up to the event
// and preserving information leading up to the issue
// for instance, log level is Error and trigger on Fatal so if Fatal
// happens then everything including trace would be dumped.
// The backlog depth is how much to maintain in memory
type Log struct {
	level             LogLevel
	triggerLevel      LogLevel
	backlogDepth      int
	nextEntry         int
	firstEntry        int
	compactDuplicates bool
	lastLog           *Entry
	duplicateCount    int
	bufferedMessages  []*Entry
	outputTargets     []io.Writer
	outputFormatters  []LogFormatter
	logLock           sync.Mutex
	tags              []*Tag
}

// New returns a new instance of a log object
func New() *Log {
	return &Log{
		level:             Error,
		triggerLevel:      Fatal,
		backlogDepth:      DefaultBacklogDepth,
		nextEntry:         0,
		firstEntry:        0,
		lastLog:           nil,
		duplicateCount:    0,
		compactDuplicates: true,
		bufferedMessages:  make([]*Entry, DefaultBacklogDepth),
		outputTargets:     make([]io.Writer, 0),
		outputFormatters:  make([]LogFormatter, 0),
		tags:              make([]*Tag, 0),
	}
}

// Clone returns a clone of the given log allowing for the cascading of tags
// this will need some thought
func (l *Log) Clone() *Log {
	newLog := New()

	newLog.level = l.level
	newLog.triggerLevel = l.triggerLevel
	newLog.backlogDepth = l.backlogDepth
	newLog.compactDuplicates = l.compactDuplicates
	newLog.outputTargets = append(newLog.outputTargets, l.outputTargets...)
	newLog.outputFormatters = append(newLog.outputFormatters, l.outputFormatters...)
	newLog.tags = append(newLog.tags, l.tags...)

	return newLog
}

// SetLevel sets the level to log at which should be less than equal to the trigger level
func (l *Log) SetLevel(level LogLevel) error {
	l.logLock.Lock()
	defer l.logLock.Unlock()

	if level > l.triggerLevel {
		return fmt.Errorf("unable to set level to higher than trigger level")
	}
	if level > Fatal {
		return fmt.Errorf("log level is out of range: %d", level)
	}
	l.level = level
	return nil
}

// SetTriggerLevel sets the level to trigger at which should be equal to or greater than the default
// log level
func (l *Log) SetTriggerLevel(triggerLevel LogLevel) error {
	l.logLock.Lock()
	defer l.logLock.Unlock()

	if triggerLevel < l.level {
		return fmt.Errorf("unable to set trigger level to less than log level")
	}

	if triggerLevel > Fatal {
		return fmt.Errorf("trigger level is out of range: %d", triggerLevel)
	}
	l.triggerLevel = triggerLevel
	return nil
}

// SetBacklogDepth sets the depth of the backlog i.e.
// how many entries excluding duplicates that are stored
func (l *Log) SetBacklogDepth(depth int) error {
	l.logLock.Lock()
	defer l.logLock.Unlock()

	if depth < 0 {
		return fmt.Errorf("bad backlog depth selected: %d", depth)
	}
	l.backlogDepth = depth
	l.bufferedMessages = make([]*Entry, depth)
	l.firstEntry = 0
	l.nextEntry = 0

	return nil
}

// SetCompactDuplicates will detect duplicate entries
// and will note the number appearing opposed to storing each one
func (l *Log) SetCompactDuplicates(compact bool) {
	l.logLock.Lock()
	defer l.logLock.Unlock()

	l.compactDuplicates = compact
}

// GetCompactDuplicates returns wheterh the logger is
// compacting duplicte entries or not
func (l *Log) GetCompactDuplicates() bool {
	return l.compactDuplicates
}

// AddOutputTarget adds an output target to dump to
// and defaults to the standard text formatter
func (l *Log) AddOutputTarget(writer io.Writer) int {
	// Lock in add target/formatter
	l.AddOutputTargetAndFormatter(writer, &TextFormatter{timeFormat: time.ANSIC})

	// index is 1 less than length
	return len(l.outputTargets) - 1
}

// AddOutputTargetAndFormatter assigns both an output
// target and the formatter for it
func (l *Log) AddOutputTargetAndFormatter(writer io.Writer, formatter LogFormatter) int {
	l.logLock.Lock()
	defer l.logLock.Unlock()

	l.outputTargets = append(l.outputTargets, writer)
	l.outputFormatters = append(l.outputFormatters, formatter)

	// index is 1 less than length
	return len(l.outputTargets) - 1
}

// SetOutputFormatter sets a specific output formatter for the given output target
func (l *Log) SetOutputFormatter(index int, formatter LogFormatter) error {
	l.logLock.Lock()
	defer l.logLock.Unlock()

	if index < 0 || index >= len(l.outputTargets) {
		return fmt.Errorf("bad output formatter index")
	}
	l.outputFormatters[index] = formatter
	return nil
}

// GetOutputFormatter returns the specified indexed output formatter
func (l *Log) GetOutputFormatter(index int) (*LogFormatter, error) {
	// make sure the underlying formatters etc are not changing
	l.logLock.Lock()
	defer l.logLock.Unlock()

	if index < 0 || index >= len(l.outputTargets) {
		return nil, fmt.Errorf("bad output formatter index")
	}
	return &l.outputFormatters[index], nil
}

// AddTag adds a give tag to a logger
func (l *Log) AddTag(name string, value interface{}) {
	l.tags = append(l.tags, CreateTag(name, value))
}

// Log will log the given string at the specified level
func (l *Log) Log(level LogLevel, message string) {
	l.logLock.Lock()
	defer l.logLock.Unlock()

	logEntry := NewEntry(level, time.Now(), message, l.tags)

	// buffer everything
	l.buffer(logEntry)

	if level >= l.level {
		// Dump lead up if triggered
		if level >= l.triggerLevel {
			// if at or above trigger level, this entry
			// has been dumpped when the buffer is dumped.
			l.dumpBuffer()
		} else {
			// output information
			for index, logger := range l.outputTargets {
				logMessage := l.outputFormatters[index].Format(logEntry)
				_, err := logger.Write(logMessage)
				if err != nil {
					fmt.Printf("failed to write to logger: %d", index)
				}
			}
		}
	}
}

//------------------------
// For the following functions
// locked in log function
//------------------------

// Logf will log the given string w/ arguments at the specified level
func (l *Log) Logf(level LogLevel, logFormat string, args ...interface{}) {
	message := fmt.Sprintf(logFormat, args...)

	l.Log(level, message)
}

// Debug helper function to reduce having to pass in the level
func (l *Log) Debug(message string) {
	l.Log(Debug, message)
}

// Debugf helper function to reduce having to pass in the level
func (l *Log) Debugf(logFormat string, args ...interface{}) {
	l.Logf(Debug, logFormat, args...)
}

// Trace helper function to reduce having to pass in the level
func (l *Log) Trace(message string) {
	l.Log(Trace, message)
}

// Tracef helper function to reduce having to pass in the level
func (l *Log) Tracef(logFormat string, args ...interface{}) {
	l.Logf(Trace, logFormat, args...)
}

// Information helper function to reduce having to pass in the level
func (l *Log) Information(message string) {
	l.Log(Information, message)
}

// Informationf helper function to reduce having to pass in the level
func (l *Log) Informationf(logFormat string, args ...interface{}) {
	l.Logf(Information, logFormat, args...)
}

// Warning helper function to reduce having to pass in the level
func (l *Log) Warning(message string) {
	l.Log(Warning, message)
}

// Warningf helper function to reduce having to pass in the level
func (l *Log) Warningf(logFormat string, args ...interface{}) {
	l.Logf(Warning, logFormat, args...)
}

// Error helper function to reduce having to pass in the level
func (l *Log) Error(message string) {
	l.Log(Error, message)
}

// Errorf helper function to reduce having to pass in the level
func (l *Log) Errorf(logFormat string, args ...interface{}) {
	l.Logf(Error, logFormat, args...)
}

// Fatal helper function to reduce having to pass in the level
func (l *Log) Fatal(message string) {
	l.Log(Fatal, message)
}

// Fatalf helper function to reduce having to pass in the level
func (l *Log) Fatalf(logFormat string, args ...interface{}) {
	l.Logf(Fatal, logFormat, args...)
}

func convertLevelToString(level LogLevel, capitalize bool) (string, error) {
	var stringLevel string
	switch level {
	case Trace:
		stringLevel = LogLevelTrace
	case Debug:
		stringLevel = LogLevelDebug
	case Information:
		stringLevel = LogLevelInformation
	case Warning:
		stringLevel = LogLevelWarning
	case Error:
		stringLevel = LogLevelError
	case Fatal:
		stringLevel = LogLevelFatal
	default:
		return "", fmt.Errorf("unable to convert level: %d", level)
	}

	if capitalize {
		stringLevel = strings.ToUpper(stringLevel)
	}

	return stringLevel, nil
}

func convertStringToLevel(level string) LogLevel {
	switch strings.ToLower(level) {
	case LogLevelTrace:
		return Trace
	case LogLevelDebug:
		return Debug
	case LogLevelInformation:
		return Information
	case LogLevelWarning:
		return Warning
	case LogLevelError:
		return Error
	case LogLevelFatal:
		return Fatal
	}
	return Error
}

func (l *Log) dumpBufferRange(entries []*Entry) {
	for _, entry := range entries {
		for index, logger := range l.outputTargets {
			logMessage := l.outputFormatters[index].Format(entry)
			_, err := logger.Write(logMessage)
			if err != nil {
				fmt.Printf("failed to write to log index: %d", index)
			}
		}
	}
}

func (l *Log) dumpBuffer() {
	// any entries?
	if l.firstEntry != l.nextEntry {
		if l.nextEntry > l.firstEntry {
			// dump the whole range
			l.dumpBufferRange(l.bufferedMessages[l.firstEntry:l.nextEntry])
		} else {
			l.dumpBufferRange(l.bufferedMessages[l.firstEntry:l.backlogDepth])
			l.dumpBufferRange(l.bufferedMessages[:l.nextEntry])
		}

		// reset the buffer once dumped
		l.bufferedMessages = make([]*Entry, l.backlogDepth)
		l.firstEntry = 0
		l.nextEntry = 0
	}
}

func (l *Log) addBufferEntry(logEntry *Entry) {
	// if next entry is rolling then set this up
	if l.nextEntry >= l.backlogDepth {
		l.nextEntry = 0
		l.firstEntry = (l.firstEntry + 1) % l.backlogDepth
	}

	// add this one in
	l.bufferedMessages[l.nextEntry] = logEntry

	l.nextEntry += 1
}

func (l *Log) buffer(logEntry *Entry) {
	isDuplicate := false
	if l.compactDuplicates {
		if l.lastLog != nil {
			if logEntry.message == l.lastLog.message {
				isDuplicate = true
				l.duplicateCount++
			} else {
				// dump the last log and count
				if l.duplicateCount > 1 {
					lastEntry := NewEntry(l.lastLog.level, l.lastLog.timestamp, l.lastLog.message+" ("+fmt.Sprintf("%d", l.duplicateCount)+")", l.tags)
					l.addBufferEntry(lastEntry)
				}
				l.duplicateCount = 0
			}
		}
		l.lastLog = logEntry
	}
	if !isDuplicate {
		l.addBufferEntry(logEntry)
	}
}
