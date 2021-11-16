package git

import (
	"encoding/hex"
	"errors"
)

// OID represents the SHA-1 object ID of a Git object, in binary
// format.
type OID struct {
	v [20]byte
}

// NullOID is the null object ID; i.e., all zeros.
var NullOID OID

// OIDFromBytes converts a byte slice containing an object ID in
// binary format into an `OID`.
func OIDFromBytes(oidBytes []byte) (OID, error) {
	var oid OID
	if len(oidBytes) != len(oid.v) {
		return OID{}, errors.New("bytes oid has the wrong length")
	}
	copy(oid.v[0:20], oidBytes)
	return oid, nil
}

// NewOID converts an object ID in hex format (i.e., `[0-9a-f]{40}`)
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
	return hex.EncodeToString(oid.v[:])
}

// Bytes returns a byte slice view of `oid`, in binary format.
func (oid OID) Bytes() []byte {
	return oid.v[:]
}

// MarshalJSON expresses `oid` as a JSON string with its enclosing
// quotation marks.
func (oid OID) MarshalJSON() ([]byte, error) {
	src := oid.v[:]
	dst := make([]byte, hex.EncodedLen(len(src))+2)
	dst[0] = '"'
	dst[len(dst)-1] = '"'
	hex.Encode(dst[1:len(dst)-1], src)
	return dst, nil
}
