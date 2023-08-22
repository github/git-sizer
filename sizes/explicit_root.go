package sizes

import "github.com/github/git-sizer/git"

type ExplicitRoot struct {
	name string
	oid  git.OID
}

func NewExplicitRoot(name string, oid git.OID) ExplicitRoot {
	return ExplicitRoot{
		name: name,
		oid:  oid,
	}
}

func (er ExplicitRoot) Name() string { return er.name }
func (er ExplicitRoot) OID() git.OID { return er.oid }
func (er ExplicitRoot) Walk() bool   { return true }
