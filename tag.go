// Package pflog defines all of the pflog package
package pflog

// Tag is a struct regarding tags that can be attached to a log
type Tag struct {
	name  string
	value interface{}
}

// CreateTag creates a new tag to track
func CreateTag(name string, value interface{}) *Tag {
	return &Tag{name: name, value: value}
}
