package msgpack

import (
	"errors"
	"fmt"
	"reflect"

	"quad4/msgpack/v5/pkg/msgpack/msgpcode"
)

var errArrayStruct = errors.New("msgpack: number of fields in array-encoded struct has changed")

var (
	mapStringStringPtrType = reflect.TypeFor[*map[string]string]()
	mapStringStringType    = mapStringStringPtrType.Elem()
	mapStringBoolPtrType   = reflect.TypeFor[*map[string]bool]()
	mapStringBoolType      = mapStringBoolPtrType.Elem()
)

var (
	mapStringInterfacePtrType = reflect.TypeFor[*map[string]any]()
	mapStringInterfaceType    = mapStringInterfacePtrType.Elem()
)

func decodeMapValue(d *Decoder, v reflect.Value) error {
	n, err := d.DecodeMapLen()
	if err != nil {
		return err
	}

	typ := v.Type()
	if n == -1 {
		v.Set(reflect.Zero(typ))
		return nil
	}

	if v.IsNil() {
		ln := n
		if d.flags&disableAllocLimitFlag == 0 {
			ln = min(ln, maxMapSize)
		}
		v.Set(reflect.MakeMapWithSize(typ, ln))
	}
	if n == 0 {
		return nil
	}

	return d.decodeTypedMapValue(v, n)
}

func (d *Decoder) decodeMapDefault() (any, error) {
	if d.mapDecoder != nil {
		return d.mapDecoder(d)
	}
	return d.DecodeMap()
}

// DecodeMapLen decodes map length. Length is -1 when map is nil.
func (d *Decoder) DecodeMapLen() (int, error) {
	c, err := d.readCode()
	if err != nil {
		return 0, err
	}

	if msgpcode.IsExt(c) {
		if err = d.skipExtHeader(c); err != nil {
			return 0, err
		}

		c, err = d.readCode()
		if err != nil {
			return 0, err
		}
	}
	return d.mapLen(c)
}

func (d *Decoder) mapLen(c byte) (int, error) {
	if c == msgpcode.Nil {
		return -1, nil
	}
	if c >= msgpcode.FixedMapLow && c <= msgpcode.FixedMapHigh {
		n := int(c & msgpcode.FixedMapMask)
		if err := d.rejectOversizedContainer(n, 2, "map"); err != nil {
			return 0, err
		}
		return n, nil
	}
	if c == msgpcode.Map16 {
		size, err := d.uint16()
		if err != nil {
			return 0, err
		}
		if err := d.rejectOversizedContainer(int(size), 2, "map"); err != nil {
			return 0, err
		}
		return int(size), nil
	}
	if c == msgpcode.Map32 {
		size, err := d.uint32()
		if err != nil {
			return 0, err
		}
		n, err := uint32ToInt(size, "map length")
		if err != nil {
			return 0, err
		}
		if err := d.rejectOversizedContainer(n, 2, "map"); err != nil {
			return 0, err
		}
		return n, nil
	}
	return 0, unexpectedCodeError{code: c, hint: "map length"}
}

func decodeMapStringStringValue(d *Decoder, v reflect.Value) error {
	mptr := v.Addr().Convert(mapStringStringPtrType).Interface().(*map[string]string)
	return d.decodeMapStringStringPtr(mptr)
}

func (d *Decoder) decodeMapStringStringPtr(ptr *map[string]string) error {
	size, err := d.DecodeMapLen()
	if err != nil {
		return err
	}
	if size == -1 {
		*ptr = nil
		return nil
	}

	m := *ptr
	if m == nil {
		ln := size
		if d.flags&disableAllocLimitFlag == 0 {
			ln = min(size, maxMapSize)
		}
		*ptr = make(map[string]string, ln)
		m = *ptr
	}

	for range size {
		mk, err := d.DecodeString()
		if err != nil {
			return err
		}
		mv, err := d.DecodeString()
		if err != nil {
			return err
		}
		m[mk] = mv
	}

	return nil
}

func decodeMapStringInterfaceValue(d *Decoder, v reflect.Value) error {
	ptr := v.Addr().Convert(mapStringInterfacePtrType).Interface().(*map[string]any)
	return d.decodeMapStringInterfacePtr(ptr)
}

func (d *Decoder) decodeMapStringInterfacePtr(ptr *map[string]any) error {
	m, err := d.DecodeMap()
	if err != nil {
		return err
	}
	*ptr = m
	return nil
}

func (d *Decoder) DecodeMap() (map[string]any, error) {
	n, err := d.DecodeMapLen()
	if err != nil {
		return nil, err
	}

	if n == -1 {
		return nil, nil
	}

	hint := n
	if d.flags&disableAllocLimitFlag == 0 {
		hint = min(hint, maxMapSize)
	}
	m := make(map[string]any, hint)

	for range n {
		mk, err := d.DecodeString()
		if err != nil {
			return nil, err
		}
		mv, err := d.decodeInterfaceCond()
		if err != nil {
			return nil, err
		}
		m[mk] = mv
	}

	return m, nil
}

func (d *Decoder) DecodeUntypedMap() (map[any]any, error) {
	n, err := d.DecodeMapLen()
	if err != nil {
		return nil, err
	}

	if n == -1 {
		return nil, nil
	}

	hint := n
	if d.flags&disableAllocLimitFlag == 0 {
		hint = min(hint, maxMapSize)
	}
	m := make(map[any]any, hint)

	for range n {
		mk, err := d.decodeInterfaceCond()
		if err != nil {
			return nil, err
		}

		mv, err := d.decodeInterfaceCond()
		if err != nil {
			return nil, err
		}

		m[mk] = mv
	}

	return m, nil
}

// DecodeTypedMap decodes a typed map. Typed map is a map that has a fixed type for keys and values.
// Key and value types may be different.
func (d *Decoder) DecodeTypedMap() (any, error) {
	n, err := d.DecodeMapLen()
	if err != nil {
		return nil, err
	}
	if n <= 0 {
		return nil, nil
	}

	key, err := d.decodeInterfaceCond()
	if err != nil {
		return nil, err
	}

	value, err := d.decodeInterfaceCond()
	if err != nil {
		return nil, err
	}

	keyType := reflect.TypeOf(key)
	valueType := reflect.TypeOf(value)

	if !keyType.Comparable() {
		return nil, fmt.Errorf("msgpack: unsupported map key: %s", keyType.String())
	}

	mapType := reflect.MapOf(keyType, valueType)

	ln := n
	if d.flags&disableAllocLimitFlag == 0 {
		ln = min(ln, maxMapSize)
	}

	mapValue := reflect.MakeMapWithSize(mapType, ln)
	mapValue.SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(value))

	n--
	if err := d.decodeTypedMapValue(mapValue, n); err != nil {
		return nil, err
	}

	return mapValue.Interface(), nil
}

func (d *Decoder) decodeTypedMapValue(v reflect.Value, n int) error {
	var (
		typ       = v.Type()
		keyType   = typ.Key()
		valueType = typ.Elem()
	)
	for range n {
		mk := d.newValue(keyType).Elem()
		if err := d.DecodeValue(mk); err != nil {
			return err
		}

		mv := d.newValue(valueType).Elem()
		if err := d.DecodeValue(mv); err != nil {
			return err
		}

		v.SetMapIndex(mk, mv)
	}

	return nil
}

func (d *Decoder) skipMap(c byte) error {
	n, err := d.mapLen(c)
	if err != nil {
		return err
	}
	for range n {
		if err := d.Skip(); err != nil {
			return err
		}
		if err := d.Skip(); err != nil {
			return err
		}
	}
	return nil
}

func decodeStructValue(d *Decoder, v reflect.Value) error {
	c, err := d.readCode()
	if err != nil {
		return err
	}

	n, err := d.mapLen(c)
	if err == nil {
		return d.decodeStruct(v, n)
	}

	var err2 error
	n, err2 = d.arrayLen(c)
	if err2 != nil {
		return err
	}

	if n <= 0 {
		v.Set(reflect.Zero(v.Type()))
		return nil
	}

	fields := structs.Fields(v.Type(), d.structTag)
	if n != len(fields.List) {
		return errArrayStruct
	}

	for _, f := range fields.List {
		if err := f.DecodeValue(d, v); err != nil {
			return err
		}
	}

	return nil
}

func (d *Decoder) decodeStruct(v reflect.Value, n int) error {
	if n == -1 {
		v.Set(reflect.Zero(v.Type()))
		return nil
	}

	fields := structs.Fields(v.Type(), d.structTag)
	for range n {
		name, err := d.decodeStringTemp()
		if err != nil {
			return err
		}

		if f := fields.Map[name]; f != nil {
			if err := f.DecodeValue(d, v); err != nil {
				return err
			}
			continue
		}

		if d.flags&disallowUnknownFieldsFlag != 0 {
			return fmt.Errorf("msgpack: unknown field %q", name)
		}
		if err := d.Skip(); err != nil {
			return err
		}
	}

	return nil
}
