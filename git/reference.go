package git

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/github/git-sizer/counts"
)

// Reference represents a Git reference.
type Reference struct {
	// Refname is the full reference name of the reference.
	Refname string

	// ObjectType is the type of the object referenced.
	ObjectType ObjectType

	// ObjectSize is the size of the referred-to object, in bytes.
	ObjectSize counts.Count32

	// OID is the OID of the referred-to object.
	OID OID
}

// ParseReference parses `line` (a non-LF-terminated line) into a
// `Reference`. It is assumed that `line` is formatted like the output
// of
//
//	git for-each-ref --format='%(objectname) %(objecttype) %(objectsize) %(refname)'
func ParseReference(line string) (Reference, error) {
	words := strings.Split(line, " ")
	if len(words) != 4 {
		return Reference{}, fmt.Errorf("line improperly formatted: %#v", line)
	}
	oid, err := NewOID(words[0])
	if err != nil {
		return Reference{}, fmt.Errorf("SHA-1 improperly formatted: %#v", words[0])
	}
	objectType := ObjectType(words[1])
	objectSize, err := strconv.ParseUint(words[2], 10, 32)
	if err != nil {
		return Reference{}, fmt.Errorf("object size improperly formatted: %#v", words[2])
	}
	refname := words[3]
	return Reference{
		Refname:    refname,
		ObjectType: objectType,
		ObjectSize: counts.Count32(objectSize),
		OID:        oid,
	}, nil
}
