package git

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/github/git-sizer/counts"
)

type BatchHeader struct {
	OID        OID
	ObjectType ObjectType
	ObjectSize counts.Count32
}

var missingHeader = BatchHeader{
	ObjectType: "missing",
}

// Parse a `cat-file --batch[-check]` output header line (including
// the trailing LF). `spec`, if not "", is used in error messages.
func ParseBatchHeader(spec string, header string) (BatchHeader, error) {
	header = header[:len(header)-1]
	words := strings.Split(header, " ")
	if words[len(words)-1] == "missing" {
		if spec == "" {
			spec = words[0]
		}
		return missingHeader, fmt.Errorf("missing object %s", spec)
	}

	oid, err := NewOID(words[0])
	if err != nil {
		return missingHeader, err
	}

	size, err := strconv.ParseUint(words[2], 10, 0)
	if err != nil {
		return missingHeader, err
	}
	return BatchHeader{
		OID:        oid,
		ObjectType: ObjectType(words[1]),
		ObjectSize: counts.NewCount32(size),
	}, nil
}
