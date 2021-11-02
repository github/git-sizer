package git

import (
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
