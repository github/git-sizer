package git

import (
	"bytes"
	//nolint:gosec // Git indeed does use SHA-1, still
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

const (
	HashSizeSHA256 = sha256.Size
	HashSizeSHA1   = sha1.Size
	HashSizeMax    = HashSizeSHA256
)

type HashAlgo int

const (
	HashUnknown HashAlgo = iota
	HashSHA1
	HashSHA256
)

// OID represents the SHA-1 object ID of a Git object, in binary
// format.
type OID struct {
	v        [HashSizeMax]byte
	hashSize int
}

func (h HashAlgo) NullOID() OID {
	switch h {
	case HashSHA1:
		return OID{hashSize: HashSizeSHA1}
	case HashSHA256:
		return OID{hashSize: HashSizeSHA256}
	}
	return OID{}
}

func (h HashAlgo) HashSize() int {
	switch h {
	case HashSHA1:
		return HashSizeSHA1
	case HashSHA256:
		return HashSizeSHA256
	}
	return 0
}

// defaultNullOID is the null object ID; i.e., all zeros.
var defaultNullOID OID

func IsNullOID(o OID) bool {
	return bytes.Equal(o.v[:], defaultNullOID.v[:])
}

// OIDFromBytes converts a byte slice containing an object ID in
// binary format into an `OID`.
func OIDFromBytes(oidBytes []byte) (OID, error) {
	var oid OID
	oidSize := len(oidBytes)
	if oidSize != HashSizeSHA1 && oidSize != HashSizeSHA256 {
		return OID{}, errors.New("bytes oid has the wrong length")
	}
	oid.hashSize = oidSize
	copy(oid.v[0:oidSize], oidBytes)
	return oid, nil
}

// NewOID converts an object ID in hex format (i.e., `[0-9a-f]{40,64}`)
// into an `OID`.
func NewOID(s string) (OID, error) {
	oidBytes, err := hex.DecodeString(s)
	if err != nil {
		return OID{}, err
	}
	return OIDFromBytes(oidBytes)
}

// String formats `oid` as a string in hex format.
func (oid OID) String() string {
	return hex.EncodeToString(oid.v[:oid.hashSize])
}

// Bytes returns a byte slice view of `oid`, in binary format.
func (oid OID) Bytes() []byte {
	return oid.v[:oid.hashSize]
}

// MarshalJSON expresses `oid` as a JSON string with its enclosing
// quotation marks.
func (oid OID) MarshalJSON() ([]byte, error) {
	src := oid.v[:oid.hashSize]
	dst := make([]byte, hex.EncodedLen(len(src))+2)
	dst[0] = '"'
	dst[len(dst)-1] = '"'
	hex.Encode(dst[1:len(dst)-1], src)
	return dst, nil
}
