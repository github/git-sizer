package git

import (
	"bytes"
	"fmt"
)

func (repo *Repository) ResolveObject(name string) (OID, error) {
	cmd := repo.GitCommand("rev-parse", "--verify", "--end-of-options", name)
	output, err := cmd.Output()
	if err != nil {
		return NullOID, fmt.Errorf("resolving object %q: %w", name, err)
	}
	oidString := string(bytes.TrimSpace(output))
	oid, err := NewOID(oidString)
	if err != nil {
		return NullOID, fmt.Errorf("parsing output %q from 'rev-parse': %w", oidString, err)
	}
	return oid, nil
}
