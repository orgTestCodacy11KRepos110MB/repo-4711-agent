// Package rivertags decodes a struct type into river object
// and structural tags.
package rivertags

import (
	"fmt"
	"reflect"
	"strings"
)

// Flags is a bitmap of flags associated with a field on a struct.
type Flags uint

// Valid flags.
const (
	FlagAttr  Flags = 1 << iota // FlagAttr treats a field as attribute
	FlagBlock                   // FlagBlock treats a field as a block

	FlagOptional // FlagOptional marks a field optional for decoding/encoding
	FlagLabel    // FlagLabel will store block labels in the field
)

// String returns the flags as a string.
func (f Flags) String() string {
	attrs := make([]string, 0, 5)

	if f&FlagAttr != 0 {
		attrs = append(attrs, "attr")
	}
	if f&FlagBlock != 0 {
		attrs = append(attrs, "block")
	}
	if f&FlagOptional != 0 {
		attrs = append(attrs, "optional")
	}
	if f&FlagLabel != 0 {
		attrs = append(attrs, "label")
	}

	return fmt.Sprintf("Flags(%s)", strings.Join(attrs, ","))
}

// GoString returns the %#v format of Flags.
func (f Flags) GoString() string { return f.String() }

// Field is a tagged field within a struct.
type Field struct {
	Name  []string // Name of tagged field
	Index []int    // Index into field (reflect.Value.FieldByIndex)
	Flags Flags    // Flags assigned to field
}

// IsAttr returns whether f is for an attribute.
func (f Field) IsAttr() bool { return f.Flags&FlagAttr != 0 }

// IsBlock returns whether f is for a block.
func (f Field) IsBlock() bool { return f.Flags&FlagBlock != 0 }

// IsOptional returns whether f is optional.
func (f Field) IsOptional() bool { return f.Flags&FlagOptional != 0 }

// Get returns the list of tagged fields for some struct type ty. Get panics if
// ty is not a struct type.
//
// Get examines each tagged field in ty for a river key. The river key is then
// parsed as containing a name for the field, followed by a required
// comma-separated list of options. The name may be empty for fields which do
// not require a name. Get will ignore any field that is not tagged with a
// river key.
//
// Get will treat anonymous struct fields as if the inner fields were fields in
// the outer struct.
//
// Examples of struct field tags and their meanings:
//
//     // Field is used as a required block named "my_block".
//     Field struct{} `river:"my_block,block"`
//
//     // Field is used as an optional block named "my_block".
//     Field struct{} `river:"my_block,block,optional"`
//
//     // Field is used as a required attribute named "my_attr".
//     Field string `river:"my_attr,attr"`
//
//     // Field is used as an optional attribute named "my_attr".
//     Field string `river:"my_attr,attr,optional"`
//
//     // Field is used for storing the label of the block which the struct
//     // represents.
//     Field string `river:",label"`
//
// With the exception of the `river:",label"` tag, all tagged fields must have a
// unique name.
//
// The type of tagged fields may be any Go type, with the exception of
// `river:",label"` tags, which must be strings.
func Get(ty reflect.Type) []Field {
	if k := ty.Kind(); k != reflect.Struct {
		panic(fmt.Sprintf("rivertags: Get requires struct kind, got %s", k))
	}

	var (
		fields []Field

		usedNames      = make(map[string][]int)
		usedLabelField = []int(nil)
	)

	for _, field := range reflect.VisibleFields(ty) {
		// River does not support embedding of fields
		if field.Anonymous {
			panic(fmt.Sprintf("river: anonymous fields not supported %s", printPathToField(ty, field.Index)))
		}

		tag, tagged := field.Tag.Lookup("river")
		if !tagged {
			continue
		}

		if !field.IsExported() {
			panic(fmt.Sprintf("river: river tag found on unexported field at %s", printPathToField(ty, field.Index)))
		}

		options := strings.SplitN(tag, ",", 2)
		if len(options) == 0 {
			panic(fmt.Sprintf("river: unsupported empty tag at %s", printPathToField(ty, field.Index)))
		}
		if len(options) != 2 {
			panic(fmt.Sprintf("river: field %s tag is missing options", printPathToField(ty, field.Index)))
		}

		fullName := options[0]

		tf := Field{
			Name:  strings.Split(fullName, "."),
			Index: field.Index,
		}

		if first, used := usedNames[fullName]; used && fullName != "" {
			panic(fmt.Sprintf("river: field name %s already used by %s", fullName, printPathToField(ty, first)))
		}
		usedNames[fullName] = tf.Index

		switch options[1] {
		case "attr":
			tf.Flags |= FlagAttr
		case "attr,optional":
			tf.Flags |= FlagAttr | FlagOptional
		case "block":
			tf.Flags |= FlagBlock
		case "block,optional":
			tf.Flags |= FlagBlock | FlagOptional
		case "label":
			tf.Flags |= FlagLabel
		default:
			panic(fmt.Sprintf("river: unrecognized river tag format %q at %s", tag, printPathToField(ty, tf.Index)))
		}

		if len(tf.Name) > 1 && tf.Flags&FlagBlock == 0 {
			panic(fmt.Sprintf("river: field names with `.` may only be used by blocks (found at %s)", printPathToField(ty, tf.Index)))
		}

		if tf.Flags&FlagLabel != 0 {
			if fullName != "" {
				panic(fmt.Sprintf("river: label field at %s must not have a name", printPathToField(ty, tf.Index)))
			}
			if field.Type.Kind() != reflect.String {
				panic(fmt.Sprintf("river: label field at %s must be a string", printPathToField(ty, tf.Index)))
			}

			if usedLabelField != nil {
				panic(fmt.Sprintf("river: label field already used by %s", printPathToField(ty, tf.Index)))
			}
			usedLabelField = tf.Index
		}

		if fullName == "" && tf.Flags&FlagLabel == 0 /* (e.g., *not* a label) */ {
			panic(fmt.Sprintf("river: non-empty field name required at %s", printPathToField(ty, tf.Index)))
		}

		fields = append(fields, tf)
	}

	return fields
}

func HasRiverTags(ty reflect.Type) bool {
	if k := ty.Kind(); k != reflect.Struct {
		return false
	}

	for _, field := range reflect.VisibleFields(ty) {
		// River does not support embedding of fields
		if field.Anonymous {
			panic(fmt.Sprintf("river: anonymous fields not supported %s", printPathToField(ty, field.Index)))
		}

		_, tagged := field.Tag.Lookup("river")
		if !tagged {
			continue
		}
		return true

	}

	return false
}

func printPathToField(structTy reflect.Type, path []int) string {
	var sb strings.Builder

	sb.WriteString(structTy.String())
	sb.WriteString(".")

	cur := structTy
	for i, elem := range path {
		sb.WriteString(cur.Field(elem).Name)

		if i+1 < len(path) {
			sb.WriteString(".")
		}

		cur = cur.Field(i).Type
	}

	return sb.String()
}
