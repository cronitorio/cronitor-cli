package cmd

import (
	"reflect"
	"strings"
	"unsafe"

	"github.com/spf13/viper"
)

// viperUnset removes keys from viper's explicit override map so AutomaticEnv
// and flags can win. Tests that claim to exercise real env/flag layers must
// call this; viper.Set shadows both.
func viperUnset(keys ...string) {
	rv := reflect.ValueOf(viper.GetViper()).Elem()
	field := rv.FieldByName("override")
	if !field.IsValid() {
		panic("viper.override not found")
	}
	m := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface().(map[string]interface{})
	for _, k := range keys {
		delete(m, strings.ToLower(k))
	}
}
