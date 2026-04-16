package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Get retrieves a config value by dot-separated path (e.g., "source.language").
// Field names are matched case-insensitively against their yaml tags.
func (c *Config) Get(dotPath string) (interface{}, error) {
	parts := strings.Split(dotPath, ".")
	if len(parts) == 0 || dotPath == "" {
		return nil, fmt.Errorf("empty dot path")
	}

	v := reflect.ValueOf(c).Elem()
	for i, part := range parts {
		v = dereferenceValue(v)
		if v.Kind() != reflect.Struct {
			return nil, fmt.Errorf("path segment %q (at position %d) is not a struct", parts[i-1], i-1)
		}
		idx, ok := findFieldByYAMLTag(v.Type(), part)
		if !ok {
			return nil, fmt.Errorf("unknown config key %q in path %q", part, dotPath)
		}
		v = v.Field(idx)
	}

	return v.Interface(), nil
}

// Set sets a config value by dot-separated path.
// It performs automatic type coercion from string to the target type.
func (c *Config) Set(dotPath string, value interface{}) error {
	parts := strings.Split(dotPath, ".")
	if len(parts) == 0 || dotPath == "" {
		return fmt.Errorf("empty dot path")
	}

	v := reflect.ValueOf(c).Elem()
	for i, part := range parts {
		v = dereferenceValue(v)
		if v.Kind() != reflect.Struct {
			return fmt.Errorf("path segment %q (at position %d) is not a struct", parts[i-1], i-1)
		}
		idx, ok := findFieldByYAMLTag(v.Type(), part)
		if !ok {
			return fmt.Errorf("unknown config key %q in path %q", part, dotPath)
		}
		v = v.Field(idx)
	}

	if !v.CanSet() {
		return fmt.Errorf("cannot set field at path %q", dotPath)
	}

	return coerceAndSet(v, value, dotPath)
}

// GetAllKeys returns all available dot-path keys suitable for tab completion.
func (c *Config) GetAllKeys() []string {
	var keys []string
	collectKeys(reflect.TypeOf(*c), "", &keys)
	return keys
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

func dereferenceValue(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return v
		}
		v = v.Elem()
	}
	return v
}

func findFieldByYAMLTag(t reflect.Type, name string) (int, bool) {
	lower := strings.ToLower(name)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("yaml")
		if tag == "" || tag == "-" {
			tag = f.Name
		}
		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}
		if strings.ToLower(tag) == lower {
			return i, true
		}
	}
	return 0, false
}

func coerceAndSet(target reflect.Value, value interface{}, dotPath string) error {
	targetType := target.Type()

	if value != nil {
		rv := reflect.ValueOf(value)
		if rv.Type().AssignableTo(targetType) {
			target.Set(rv)
			return nil
		}
		if rv.Type().ConvertibleTo(targetType) {
			target.Set(rv.Convert(targetType))
			return nil
		}
	}

	strVal, isString := value.(string)
	if !isString {
		return fmt.Errorf("cannot assign %T to field %q of type %s", value, dotPath, targetType)
	}

	switch targetType.Kind() {
	case reflect.String:
		target.SetString(strVal)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(strVal, 10, 64)
		if err != nil {
			return fmt.Errorf("cannot convert %q to int for field %q: %w", strVal, dotPath, err)
		}
		target.SetInt(n)
	case reflect.Bool:
		b, err := strconv.ParseBool(strVal)
		if err != nil {
			return fmt.Errorf("cannot convert %q to bool for field %q: %w", strVal, dotPath, err)
		}
		target.SetBool(b)
	case reflect.Slice:
		slicePtr := reflect.New(targetType)
		if err := json.Unmarshal([]byte(strVal), slicePtr.Interface()); err != nil {
			return fmt.Errorf("cannot parse %q as JSON array for field %q: %w", strVal, dotPath, err)
		}
		target.Set(slicePtr.Elem())
	default:
		return fmt.Errorf("unsupported target type %s for field %q", targetType, dotPath)
	}

	return nil
}

func collectKeys(t reflect.Type, prefix string, keys *[]string) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		if idx := strings.Index(tag, ","); idx != -1 {
			tag = tag[:idx]
		}

		fullPath := tag
		if prefix != "" {
			fullPath = prefix + "." + tag
		}

		ft := f.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}

		if ft.Kind() == reflect.Struct {
			collectKeys(ft, fullPath, keys)
		} else {
			*keys = append(*keys, fullPath)
		}
	}
}
