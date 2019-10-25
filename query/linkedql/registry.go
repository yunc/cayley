package linkedql

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/cayleygraph/quad"
)

var (
	typeByName = make(map[string]reflect.Type)
	nameByType = make(map[reflect.Type]string)
)

// Register adds an Item type to the registry
func Register(typ RegistryItem) {
	tp := reflect.TypeOf(typ)
	if tp.Kind() == reflect.Ptr {
		tp = tp.Elem()
	}
	if tp.Kind() != reflect.Struct {
		panic("only structs are allowed")
	}
	name := string(typ.Type())
	if _, ok := typeByName[name]; ok {
		panic("this name was already registered")
	}
	typeByName[name] = tp
	nameByType[tp] = name
}

var quadValue = reflect.TypeOf((*quad.Value)(nil)).Elem()
var quadSliceValue = reflect.TypeOf(([]quad.Value)(nil))

// Unmarshal attempts to unmarshal an Item or returns error
func Unmarshal(data []byte) (RegistryItem, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	var typ string
	if err := json.Unmarshal(m["@type"], &typ); err != nil {
		return nil, err
	}
	delete(m, "@type")
	tp, ok := typeByName[typ]
	if !ok {
		return nil, fmt.Errorf("unsupported item: %q", typ)
	}
	item := reflect.New(tp).Elem()
	for i := 0; i < tp.NumField(); i++ {
		f := tp.Field(i)
		name := f.Name
		tag := strings.SplitN(f.Tag.Get("json"), ",", 2)[0]
		if tag == "-" {
			continue
		} else if tag != "" {
			name = tag
		}
		v, ok := m[name]
		if !ok {
			continue
		}
		fv := item.Field(i)
		switch f.Type {
		case quadValue:
			var a interface{}
			err := json.Unmarshal(v, &a)
			if err != nil {
				return nil, err
			}
			value, err := parseValue(v)
			if err != nil {
				return nil, err
			}
			fv.Set(reflect.ValueOf(value))
			continue
		case quadSliceValue:
			var a []interface{}
			err := json.Unmarshal(v, &a)
			if err != nil {
				return nil, err
			}
			var values []quad.Value
			for _, item := range a {
				value, err := parseValue(item)
				if err != nil {
					return nil, err
				}
				values = append(values, value)
			}
			fv.Set(reflect.ValueOf(values))
			continue
		}
		switch f.Type.Kind() {
		case reflect.Interface:
			s, err := Unmarshal(v)
			if err != nil {
				return nil, err
			}
			fv.Set(reflect.ValueOf(s))
		case reflect.Slice:
			el := f.Type.Elem()
			if el.Kind() != reflect.Interface {
				err := json.Unmarshal(v, fv.Addr().Interface())
				if err != nil {
					return nil, err
				}
			} else {
				var arr []json.RawMessage
				if err := json.Unmarshal(v, &arr); err != nil {
					return nil, err
				}
				if arr != nil {
					va := reflect.MakeSlice(f.Type, len(arr), len(arr))
					for i, v := range arr {
						s, err := Unmarshal(v)
						if err != nil {
							return nil, err
						}
						va.Index(i).Set(reflect.ValueOf(s))
					}
					fv.Set(va)
				}
			}
		default:
			err := json.Unmarshal(v, fv.Addr().Interface())
			if err != nil {
				return nil, err
			}
		}
	}
	return item.Addr().Interface().(RegistryItem), nil
}

const xsd = "http://www.w3.org/2001/XMLSchema#"
const xsdInt = xsd + "integer"
const xsdFloat = xsd + "float"
const xsdBool = xsd + "boolean"

func parseValue(a interface{}) (quad.Value, error) {
	switch a := a.(type) {
	case string:
		return quad.String(a), nil
	case int64:
		return quad.TypedString{Value: quad.String(a), Type: quad.IRI(xsdInt)}, nil
	case float64:
		return quad.TypedString{Value: quad.String(fmt.Sprintf("%f", a)), Type: quad.IRI(xsdFloat)}, nil
	case bool:
		return quad.TypedString{Value: quad.String(fmt.Sprintf("%t", a)), Type: quad.IRI(xsdBool)}, nil
	case map[string]interface{}:
		id, ok := a["@id"].(string)
		if ok {
			if strings.HasPrefix(id, "_:") {
				return quad.BNode(id[2:]), nil
			}
			return quad.IRI(id), nil
		}
		value, ok := a["@value"].(string)
		if ok {
			if language, ok := a["@language"].(string); ok {
				return quad.LangString{Value: quad.String("value"), Lang: language}, nil
			}
			if _type, ok := a["@type"].(string); ok {
				return quad.TypedString{Value: quad.String(value), Type: quad.IRI(_type)}, nil
			}
		}
	}
	return nil, fmt.Errorf("cannot parse JSON-LD value: %#v", a)
}

// RegistryItem in the registry
type RegistryItem interface {
	Type() quad.IRI
}
