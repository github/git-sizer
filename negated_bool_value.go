package main

import (
	"strconv"
)

// NegatedBoolValue is a `pflag.Value` that set a boolean variable to
// the inverse of what the argument would normally indicate (e.g., to
// implement `--no-foo`-style arguments).
type NegatedBoolValue struct {
	value *bool
}

func (v *NegatedBoolValue) Set(s string) error {
	b, err := strconv.ParseBool(s)
	*v.value = !b
	return err
}

func (v *NegatedBoolValue) Get() interface{} {
	return !*v.value
}

func (v *NegatedBoolValue) String() string {
	if v == nil || v.value == nil {
		return "true"
	}

	return strconv.FormatBool(!*v.value)
}

func (v *NegatedBoolValue) Type() string {
	return "bool"
}
