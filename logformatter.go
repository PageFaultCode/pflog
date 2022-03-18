// Package pflog defines all of the pflog package
package pflog

import "fmt"

// LogFormatter can format logs a specific way and
// can time stamp them as required, the default is no
// timestamp unless the format is supplied
type LogFormatter interface {
	ID() string
	SetTimestampFormat(format string)
	Format(entry *Entry) []byte
}

var formatters map[string]LogFormatter

// init initialized the log package and known formatters
func init() {
	formatters = make(map[string]LogFormatter)
	err := RegisterFormatter(jsonformatterTypeID, &JSONFormatter{})
	if err != nil {
		panic(err)
	}
	err = RegisterFormatter(formatterTypeID, &TextFormatter{})
	if err != nil {
		panic(err)
	}
	err = RegisterFormatter(yamlformatterTypeID, &YAMLFormatter{})
	if err != nil {
		panic(err)
	}
}

// RegisterFormatter registers a given formatter with the system prior
// to configuration loading a config file
func RegisterFormatter(id string, formatter LogFormatter) error {
	_, exists := formatters[id]
	if exists {
		return fmt.Errorf("formatter %v already exists", id)
	}
	formatters[id] = formatter
	return nil
}

// CreateFormatter returns a formatter based on the selected
// id such as read from a config file.
func CreateFormatter(id string) (LogFormatter, error) {
	formatter, exists := formatters[id]
	if !exists {
		return nil, fmt.Errorf("unable to create formatter: %v", id)
	}

	return formatter, nil
}
