package main

import (
	"strconv"
)

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
	} else {
		return strconv.FormatBool(!*v.value)
	}
}

func (v *NegatedBoolValue) Type() string {
	return "bool"
}
