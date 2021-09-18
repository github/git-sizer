package git

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigKeyMatchesPrefix(t *testing.T) {
	for _, p := range []struct {
		key, prefix    string
		expectedBool   bool
		expectedString string
	}{
		{"foo.bar", "", true, "foo.bar"},
		{"foo.bar", "foo", true, "bar"},
		{"foo.bar", "foo.", true, "bar"},
		{"foo.bar", "foo.bar", true, ""},
		{"foo.bar", "foo.bar.", false, ""},
		{"foo.bar", "foo.bar.baz", false, ""},
		{"foo.bar", "foo.barbaz", false, ""},
		{"foo.bar.baz", "foo.bar", true, "baz"},
		{"foo.barbaz", "foo.bar", false, ""},
		{"foo.bar", "bar", false, ""},
	} {
		t.Run(
			fmt.Sprintf("TestConfigKeyMatchesPrefix(%q, %q)", p.key, p.prefix),
			func(t *testing.T) {
				ok, s := configKeyMatchesPrefix(p.key, p.prefix)
				assert.Equal(t, p.expectedBool, ok)
				assert.Equal(t, p.expectedString, s)
			},
		)
	}
}
