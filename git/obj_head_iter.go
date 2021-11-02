package git

import (
	"bytes"
	"fmt"
	"strings"
)

// ObjectHeaderIter iterates over the headers within a commit or tag
// object.
type ObjectHeaderIter struct {
	name string
	data string
}

// NewObjectHeaderIter returns an `ObjectHeaderIter` that iterates
// over the headers in a commit or tag object. `data` should be the
// object's contents, which is usually terminated by a blank line that
// separates the header from the comment. However, annotated tags
// don't always include comments, and Git even tolerates commits
// without comments, so don't insist on a blank line. `name` is used
// in error messages.
func NewObjectHeaderIter(name string, data []byte) (ObjectHeaderIter, error) {
	headerEnd := bytes.Index(data, []byte("\n\n"))
	if headerEnd == -1 {
		if len(data) == 0 {
			return ObjectHeaderIter{}, fmt.Errorf("%s has zero length", name)
		}

		if data[len(data)-1] != '\n' {
			return ObjectHeaderIter{}, fmt.Errorf("%s has no terminating LF", name)
		}

		return ObjectHeaderIter{name, string(data)}, nil
	}
	return ObjectHeaderIter{name, string(data[:headerEnd+1])}, nil
}

// HasNext returns true iff there are more headers to retrieve.
func (iter *ObjectHeaderIter) HasNext() bool {
	return len(iter.data) > 0
}

// Next returns the key and value of the next header.
func (iter *ObjectHeaderIter) Next() (string, string, error) {
	if len(iter.data) == 0 {
		return "", "", fmt.Errorf("header for %s read past end", iter.name)
	}
	header := iter.data
	keyEnd := strings.IndexByte(header, ' ')
	if keyEnd == -1 {
		return "", "", fmt.Errorf("malformed header in %s", iter.name)
	}
	key := header[:keyEnd]
	header = header[keyEnd+1:]
	valueEnd := strings.IndexByte(header, '\n')
	if valueEnd == -1 {
		return "", "", fmt.Errorf("malformed header in %s", iter.name)
	}
	value := header[:valueEnd]
	iter.data = header[valueEnd+1:]
	return key, value, nil
}
