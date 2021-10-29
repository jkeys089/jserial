package jserial

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"io"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// ParseSerializedObject parses a serialized java object.
func ParseSerializedObject(buf []byte) (content []interface{}, err error) {
	option := SetMaxDataBlockSize(len(buf))
	sop := NewSerializedObjectParser(bytes.NewReader(buf), option)

	return sop.ParseSerializedObject()
}

// ParseSerializedObject parses a serialized java object from stream.
func (sop *SerializedObjectParser) ParseSerializedObject() (content []interface{}, err error) {
	if err = sop.magic(); err != nil {
		return
	}

	if err = sop.version(); err != nil {
		return
	}

	for !sop.end() {
		var nxt interface{}

		if nxt, err = sop.content(nil); err != nil {
			if errors.Cause(err).Error() == io.EOF.Error() {
				err = errors.New("premature end of input")
			}

			return
		}

		content = append(content, nxt)
	}

	return
}

// ParseSingleObject parses a serialized java object from stream.
func ParseSingleObject(r io.Reader) (c interface{}, err error) {
	sop := newUnbufferedObjectParser(r)
	if err = sop.magic(); err != nil {
		return
	}

	if err = sop.version(); err != nil {
		return
	}

	return sop.content(nil)
}

// ParseSerializedObjectMinimal parses a serialized java object and returns the minimal object representation
// (i.e. without all the class info, etc...).
func ParseSerializedObjectMinimal(buf []byte) (content []interface{}, err error) {
	if content, err = ParseSerializedObject(buf); err == nil {
		content = jsonFriendlyArray(content)
	}

	return
}

// ParseSerializedObjectMinimal parses a serialized java object from stream
// and returns the minimal object representation (i.e. without all the class info, etc...).
func (sop *SerializedObjectParser) ParseSerializedObjectMinimal() (content []interface{}, err error) {
	if content, err = sop.ParseSerializedObject(); err == nil {
		content = jsonFriendlyArray(content)
	}

	return
}

// jsonFriendlyObject recursively filters / formats object fields to be as simple / JSON-like as possible.
func jsonFriendlyObject(obj interface{}) (jsonObj interface{}) {
	if m, isMap := obj.(map[string]interface{}); isMap {
		jsonMap := jsonFriendlyMap(m)
		jsonObj = jsonMap

		// if we have a single "value" key or a post-processed value just promote the value
		if mVal, mValExists := jsonMap["value"]; mValExists {
			_, mRawExists := jsonMap["@"]
			if mRawExists || len(jsonMap) == 1 {
				jsonObj = mVal
			}
		}

		return
	}

	if arr, isArray := obj.([]interface{}); isArray {
		jsonObj = jsonFriendlyArray(arr)

		return
	}

	// default for raw / primitive fields
	return obj
}

// jsonFriendlyArray recursively filters / formats a deserialized array.
func jsonFriendlyArray(arrayObj []interface{}) (jsonArray []interface{}) {
	jsonArray = make([]interface{}, len(arrayObj))
	for idx, arrayMember := range arrayObj {
		jsonArray[idx] = jsonFriendlyObject(arrayMember)
	}

	return
}

// jsonFriendlyMap recursively filters / formats a deserialized map.
func jsonFriendlyMap(mapObj map[string]interface{}) (jsonMap map[string]interface{}) {
	jsonMap = make(map[string]interface{})

	for k, v := range mapObj {
		// filter out `extends` keyword which just contains internal inheritance hierarchy
		if k == "extends" {
			continue
		}
		// filter out internal class definitions
		if _, isClazz := v.(*clazz); !isClazz {
			jsonMap[k] = jsonFriendlyObject(v)
		}
	}

	return
}

func init() {
	knownParsers = map[string]parser{
		"Enum":          parseEnum,
		"BlockDataLong": parseBlockDataLong,
		"BlockData":     parseBlockData,
		"EndBlockData":  parseEndBlockData,
		"ClassDesc":     parseClassDesc,
		"Class":         parseClass,
		"Array":         parseArray,
		"LongString":    parseLongString,
		"String":        parseString,
		"Null":          parseNull,
		"Object":        parseObject,
		"Reference":     parseReference,
	}
}

// typeNames includes all known type names.
var typeNames = []string{
	"Null",
	"Reference",
	"ClassDesc",
	"Object",
	"String",
	"Array",
	"Class",
	"BlockData",
	"EndBlockData",
	"Reset",
	"BlockDataLong",
	"Exception",
	"LongString",
	"ProxyClassDesc",
	"Enum",
}

// typeNameMax is used to ensure an encountered type is known.
var typeNameMax = uint8(len(typeNames) - 1)

// allowedClazzNames includes all allowed names when parsing a class descriptor.
var allowedClazzNames = map[string]bool{
	"ClassDesc":      true,
	"ProxyClassDesc": true,
	"Null":           true,
	"Reference":      true,
}

// parser is a func capable of reading a single serialized type.
type parser func(sop *SerializedObjectParser) (interface{}, error)

// knownParsers maps serialized names to corresponding parser implementations.
var knownParsers map[string]parser

// PostProc handlers are used to format deserialized objects for easier consumption.
type PostProc func(map[string]interface{}, []interface{}) (map[string]interface{}, error)

// KnownPostProcs maps serialized object signatures to PostProc implementations.
var KnownPostProcs = map[string]PostProc{
	"java.util.ArrayList@7881d21d99c7619d":  listPostProc,
	"java.util.ArrayDeque@207cda2e240da08b": listPostProc,
	"java.util.Hashtable@13bb0f25214ae4b8":  mapPostProc,
	"java.util.HashMap@0507dac1c31660d1":    mapPostProc,
	"java.util.EnumMap@065d7df7be907ca1":    enumMapPostProc,
	"java.util.HashSet@ba44859596b8b734":    hashSetPostProc,
	"java.util.Date@686a81014b597419":       datePostProc,
}

// primitiveHandler are used to read primitive values.
type primitiveHandler func(sop *SerializedObjectParser) (interface{}, error)

// primitiveHandlers maps serialized primitive identifiers to a corresponding primitiveHandler.
var primitiveHandlers = map[string]primitiveHandler{
	"B": func(sop *SerializedObjectParser) (b interface{}, err error) {
		if b, err = sop.readInt8(); err != nil {
			err = errors.Wrap(err, "error reading byte primitive")
		}

		return
	},
	"C": func(sop *SerializedObjectParser) (char interface{}, err error) {
		var charCode uint16
		if charCode, err = sop.readUInt16(); err != nil {
			err = errors.Wrap(err, "error reading char primitive")
		} else {
			char = string(rune(charCode))
		}

		return
	},
	"D": func(sop *SerializedObjectParser) (double interface{}, err error) {
		if double, err = sop.readFloat64(); err != nil {
			err = errors.Wrap(err, "error reading double primitive")
		}

		return
	},
	"F": func(sop *SerializedObjectParser) (f32 interface{}, err error) {
		if f32, err = sop.readFloat32(); err != nil {
			err = errors.Wrap(err, "error reading float primitive")
		}

		return
	},
	"I": func(sop *SerializedObjectParser) (i32 interface{}, err error) {
		if i32, err = sop.readInt32(); err != nil {
			err = errors.Wrap(err, "error reading int primitive")
		}

		return
	},
	"J": func(sop *SerializedObjectParser) (long interface{}, err error) {
		if long, err = sop.readInt64(); err != nil {
			err = errors.Wrap(err, "error reading long primitive")
		}

		return
	},
	"S": func(sop *SerializedObjectParser) (short interface{}, err error) {
		if short, err = sop.readInt16(); err != nil {
			err = errors.Wrap(err, "error reading short primitive")
		}

		return
	},
	"Z": func(sop *SerializedObjectParser) (b interface{}, err error) {
		var x int8
		if x, err = sop.readInt8(); err != nil {
			err = errors.Wrap(err, "error reading boolean primitive")
		} else {
			b = x != 0
		}

		return
	},
	"L": func(sop *SerializedObjectParser) (obj interface{}, err error) {
		if obj, err = sop.content(nil); err != nil {
			err = errors.Wrap(err, "error reading object primitive")
		}

		return
	},
	"[": func(sop *SerializedObjectParser) (arr interface{}, err error) {
		if arr, err = sop.content(nil); err != nil {
			err = errors.Wrap(err, "error reading array primitive")
		}

		return
	},
}

// SerializedObjectParser reads serialized java objects
// see: https://docs.oracle.com/javase/8/docs/platform/serialization/spec/protocol.html
type SerializedObjectParser struct {
	buf              bytes.Buffer
	rd               io.Reader
	handles          []interface{}
	maxDataBlockSize int
}

const bufferSize = 1024

type Option func(sop *SerializedObjectParser)

// SetMaxDataBlockSize set the maximum size of the parsed data block,
// by default it is equal to the value of the buffer size bufio.Reader or size of bytes.Reader.
func SetMaxDataBlockSize(maxSize int) Option {
	return func(sop *SerializedObjectParser) {
		sop.maxDataBlockSize = maxSize
	}
}

// newUnbufferedObjectParser reads serialized java objects from stream.
func newUnbufferedObjectParser(rd io.Reader) *SerializedObjectParser {
	sop := &SerializedObjectParser{
		rd:               rd,
		maxDataBlockSize: bufferSize,
	}

	return sop
}

// NewSerializedObjectParser reads serialized java objects from stream.
func NewSerializedObjectParser(rd io.Reader, options ...Option) *SerializedObjectParser {
	buf := bufio.NewReaderSize(rd, bufferSize)
	sop := &SerializedObjectParser{
		rd:               buf,
		maxDataBlockSize: buf.Size(),
	}

	for _, option := range options {
		option(sop)
	}

	return sop
}

// newHandle adds a parsed object to the existing indexed handles which can be used later to lookup references to
// existing objects.
func (sop *SerializedObjectParser) newHandle(obj interface{}) interface{} {
	sop.handles = append(sop.handles, obj)

	return obj
}

// content reads the next object in the stream and parses it.
func (sop *SerializedObjectParser) content(allowedNames map[string]bool) (content interface{}, err error) {
	var tc uint8

	if tc, err = sop.readUInt8(); err != nil {
		return
	}

	const typeMask = 0x70
	tc -= typeMask

	if tc > typeNameMax {
		if br, buffered := sop.rd.(*bufio.Reader); buffered {
			// prevents reading unknown ("foreign") byte from the stream
			br.UnreadByte() //nolint:errcheck
		}

		err = errors.Errorf("unknown type %#x", tc+typeMask)

		return
	}

	name := typeNames[tc]
	if allowedNames != nil && !allowedNames[name] {
		err = errors.Errorf("%s not allowed here", name)

		return
	}

	parse, exists := knownParsers[name]
	if !exists {
		err = errors.Errorf("parsing %s is currently not supported", name)

		return
	}

	return parse(sop)
}

// end check has next byte in stream.
func (sop *SerializedObjectParser) end() bool {
	if br, buffered := sop.rd.(*bufio.Reader); buffered {
		if br.Buffered() == 0 {
			_, eof := br.Peek(1)

			return eof != nil
		}
		return false
	}
	return true
}

// readString reads a string of length cnt bytes.
func (sop *SerializedObjectParser) readString(cnt int, asHex bool) (s string, err error) {
	sop.buf.Reset()

	// Prevented to allocate an extremely large block of memory.
	if cnt > sop.maxDataBlockSize {
		err = errors.Errorf("block data exceeds size of reader buffer. " +
			"To increase the size, use the method SetMaxDataBlockSize or use bufio.Reader with a larger buffer size")

		return
	}

	if _, err = io.CopyN(&sop.buf, sop.rd, int64(cnt)); err != nil {
		err = errors.Wrap(err, "error reading string")

		return
	}

	if asHex {
		s = hex.EncodeToString(sop.buf.Bytes())
	} else {
		s = sop.buf.String()
	}

	return
}

func (sop *SerializedObjectParser) readUInt8() (x uint8, err error) {
	if err = binary.Read(sop.rd, binary.BigEndian, &x); err != nil {
		err = errors.Wrap(err, "error reading uint8")
	}

	return
}

func (sop *SerializedObjectParser) readInt8() (x int8, err error) {
	if err = binary.Read(sop.rd, binary.BigEndian, &x); err != nil {
		err = errors.Wrap(err, "error reading int8")
	}

	return
}

func (sop *SerializedObjectParser) readUInt16() (x uint16, err error) {
	if err = binary.Read(sop.rd, binary.BigEndian, &x); err != nil {
		err = errors.Wrap(err, "error reading uint16")
	}

	return
}

func (sop *SerializedObjectParser) readInt16() (x int16, err error) {
	if err = binary.Read(sop.rd, binary.BigEndian, &x); err != nil {
		err = errors.Wrap(err, "error reading int16")
	}

	return
}

func (sop *SerializedObjectParser) readUInt32() (x uint32, err error) {
	if err = binary.Read(sop.rd, binary.BigEndian, &x); err != nil {
		err = errors.Wrap(err, "error reading uint32")
	}

	return
}

func (sop *SerializedObjectParser) readInt32() (x int32, err error) {
	if err = binary.Read(sop.rd, binary.BigEndian, &x); err != nil {
		err = errors.Wrap(err, "error reading int32")
	}

	return
}

func (sop *SerializedObjectParser) readFloat32() (x float32, err error) {
	if err = binary.Read(sop.rd, binary.BigEndian, &x); err != nil {
		err = errors.Wrap(err, "error reading float32")
	}

	return
}

func (sop *SerializedObjectParser) readInt64() (x int64, err error) {
	if err = binary.Read(sop.rd, binary.BigEndian, &x); err != nil {
		err = errors.Wrap(err, "error reading int64")
	}

	return
}

func (sop *SerializedObjectParser) readFloat64() (x float64, err error) {
	if err = binary.Read(sop.rd, binary.BigEndian, &x); err != nil {
		err = errors.Wrap(err, "error reading float64")
	}

	return
}

// utf reads a variable length string.
func (sop *SerializedObjectParser) utf() (s string, err error) {
	var offset uint16

	if offset, err = sop.readUInt16(); err != nil {
		err = errors.Wrap(err, "error reading utf: unable to read segment length")

		return
	}

	if s, err = sop.readString(int(offset), false); err != nil {
		err = errors.Wrap(err, "error reading utf: unable to read segment")
	}

	return
}

// utf reads a large (up to 2^32 bytes) variable length string.
func (sop *SerializedObjectParser) utfLong() (s string, err error) {
	var offset uint32

	if offset, err = sop.readUInt32(); err != nil {
		err = errors.Wrap(err, "error reading utf: unable to read first segment length")

		return
	}

	if offset != 0 {
		err = errors.New("unable to read string larger than 2^32 bytes")

		return
	}

	if offset, err = sop.readUInt32(); err != nil {
		err = errors.Wrap(err, "error reading utf long: unable to read second segment length")

		return
	}

	if s, err = sop.readString(int(offset), false); err != nil {
		err = errors.Wrap(err, "error reading utf long: unable to read segment")
	}

	return
}

// magic checks for the presence of the STREAM_MAGIC value.
func (sop *SerializedObjectParser) magic() error {
	magicVal, err := sop.readUInt16()

	if err == nil && magicVal != 0xaced {
		return errors.New("magic value STREAM_MAGIC not found")
	}

	return err
}

// version checks to be sure the serialized object is using a supported protocol version.
func (sop *SerializedObjectParser) version() error {
	ver, err := sop.readUInt16()
	if err != nil {
		return err
	}

	const protocolVersion = 5
	if ver != protocolVersion {
		return errors.Errorf("protocol version not recognized: wanted 5 got %d", ver)
	}

	return nil
}

// field contains info about a single class member.
type field struct {
	className string
	typeName  string
	name      string
}

// fieldDesc reads a single field descriptor.
func (sop *SerializedObjectParser) fieldDesc() (f *field, err error) {
	var typeDec uint8

	if typeDec, err = sop.readUInt8(); err != nil {
		err = errors.Wrap(err, "error reading field type")

		return
	}

	var name string

	if name, err = sop.utf(); err != nil {
		err = errors.Wrap(err, "error reading field name")

		return
	}

	typeName := string(typeDec)

	f = &field{
		typeName: typeName,
		name:     name,
	}

	if strings.Contains("[L", typeName) { //nolint
		var className interface{}

		if className, err = sop.content(nil); err != nil {
			err = errors.Wrap(err, "error reading field class name")

			return
		}

		var isString bool
		if f.className, isString = className.(string); !isString {
			err = errors.New("unexpected field class name type")
		}
	}

	return
}

// annotations reads all class annotations.
func (sop *SerializedObjectParser) annotations(allowedNames map[string]bool) (anns []interface{}, err error) {
	for {
		var ann interface{}

		if ann, err = sop.content(allowedNames); err != nil {
			err = errors.Wrap(err, "error reading class annotation")

			return
		}

		if _, isEndBlock := ann.(endBlockT); isEndBlock {
			break
		}

		anns = append(anns, ann)
	}

	return
}

// clazz contains java class info.
type clazz struct {
	super            *clazz
	annotations      []interface{}
	fields           []*field
	serialVersionUID string
	name             string
	flags            uint8
	isEnum           bool
}

// classDesc reads a class descriptor.
func (sop *SerializedObjectParser) classDesc() (cls *clazz, err error) {
	var x interface{}

	if x, err = sop.content(allowedClazzNames); err != nil {
		err = errors.Wrap(err, "error reading class description")

		return
	}

	if x == nil {
		return
	}

	var isClazz bool
	if cls, isClazz = x.(*clazz); !isClazz {
		err = errors.New("unexpected type returned while reading class description")
	}

	return
}

// parseClassDesc parses a class descriptor.
//nolint:funlen
func parseClassDesc(sop *SerializedObjectParser) (x interface{}, err error) {
	cls := &clazz{}

	if cls.name, err = sop.utf(); err != nil {
		err = errors.Wrap(err, "error reading class name")

		return
	}

	const minClassNameLength = 2
	if len(cls.name) < minClassNameLength {
		err = errors.Wrapf(err, "invalid class name: '%s'", cls.name)

		return
	}

	const serialVersionUIDLength = 8
	if cls.serialVersionUID, err = sop.readString(serialVersionUIDLength, true); err != nil {
		err = errors.Wrap(err, "error reading class serialVersionUID")

		return
	}

	sop.newHandle(cls)

	if cls.flags, err = sop.readUInt8(); err != nil {
		err = errors.Wrap(err, "error reading class flags")

		return
	}

	cls.isEnum = (cls.flags & 0x10) != 0

	var fieldCount uint16

	if fieldCount, err = sop.readUInt16(); err != nil {
		err = errors.Wrap(err, "error reading class field count")

		return
	}

	for i := 0; i < int(fieldCount); i++ {
		var f *field

		if f, err = sop.fieldDesc(); err != nil {
			err = errors.Wrap(err, "error reading class field")

			return
		}

		cls.fields = append(cls.fields, f)
	}

	if cls.annotations, err = sop.annotations(nil); err != nil {
		err = errors.Wrap(err, "error reading class annotations")

		return
	}

	if cls.super, err = sop.classDesc(); err != nil {
		err = errors.Wrap(err, "error reading class super")

		return
	}

	x = cls

	return
}

func parseClass(sop *SerializedObjectParser) (cd interface{}, err error) {
	if cd, err = sop.classDesc(); err != nil {
		err = errors.Wrap(err, "error parsing class")

		return
	}

	cd = sop.newHandle(cd)

	return
}

func parseReference(sop *SerializedObjectParser) (ref interface{}, err error) {
	var refIdx int32

	if refIdx, err = sop.readInt32(); err != nil {
		err = errors.Wrap(err, "error reading reference index")

		return
	}

	const refIDMask = 0x7e0000
	i := int(refIdx - refIDMask)

	if i > -1 && i < len(sop.handles) {
		ref = sop.handles[i]
	}

	return
}

func parseArray(sop *SerializedObjectParser) (arr interface{}, err error) {
	var cls *clazz

	if cls, err = sop.classDesc(); err != nil {
		err = errors.Wrap(err, "error parsing array class")

		return
	}

	res := map[string]interface{}{
		"class": cls,
	}

	sop.newHandle(res)

	var size int32

	if size, err = sop.readInt32(); err != nil {
		err = errors.Wrap(err, "error reading array size")

		return
	}

	res["length"] = size

	if cls == nil {
		return
	}

	primHandler, exists := primitiveHandlers[string(cls.name[1])]
	if !exists {
		err = errors.Errorf("unknown field type '%s'", string(cls.name[1]))

		return
	}

	var array []interface{}

	for i := 0; i < int(size); i++ {
		var nxt interface{}

		if nxt, err = primHandler(sop); err != nil {
			err = errors.Wrap(err, "error reading primitive array member")

			return
		}

		array = append(array, nxt)
	}

	arr = array

	return
}

// newDeferredHandle reserves an object handle slot and returns a func which can set the slot value at a later time.
func (sop *SerializedObjectParser) newDeferredHandle() func(interface{}) interface{} {
	idx := len(sop.handles)
	sop.handles = append(sop.handles, nil)

	return func(obj interface{}) interface{} {
		sop.handles[idx] = obj

		return obj
	}
}

func parseEnum(sop *SerializedObjectParser) (enum interface{}, err error) {
	var cls *clazz

	if cls, err = sop.classDesc(); err != nil {
		err = errors.Wrap(err, "error parsing enum class")

		return
	}

	deferredHandle := sop.newDeferredHandle()

	var enumConstant interface{}

	if enumConstant, err = sop.content(nil); err != nil {
		err = errors.Wrap(err, "error parsing enum constant")

		return
	}

	res := map[string]interface{}{
		"value": enumConstant,
		"class": cls,
	}

	enum = deferredHandle(res)

	return
}

func parseBlockData(sop *SerializedObjectParser) (bd interface{}, err error) {
	var size uint8

	if size, err = sop.readUInt8(); err != nil {
		err = errors.Wrap(err, "error parsing block data size")

		return
	}

	data := make([]byte, size)

	if _, err = io.ReadFull(sop.rd, data); err == nil {
		bd = data
	}

	return
}

func parseBlockDataLong(sop *SerializedObjectParser) (bdl interface{}, err error) {
	var size uint32

	if size, err = sop.readUInt32(); err != nil {
		err = errors.Wrap(err, "error parsing block data long size")

		return
	}

	// Prevented to allocate an extremely large block of memory.
	if int(size) > sop.maxDataBlockSize {
		err = errors.Errorf("block data exceeds size of reader buffer. " +
			"To increase the size, use the method SetMaxDataBlockSize or use bufio.Reader with a larger buffer size")

		return
	}

	data := make([]byte, size)

	if _, err = io.ReadFull(sop.rd, data); err == nil {
		bdl = data

		return
	}

	return
}

func parseString(sop *SerializedObjectParser) (str interface{}, err error) {
	if str, err = sop.utf(); err != nil {
		err = errors.Wrap(err, "error parsing string")
	} else {
		str = sop.newHandle(str)
	}

	return
}

func parseLongString(sop *SerializedObjectParser) (longStr interface{}, err error) {
	if longStr, err = sop.utfLong(); err != nil {
		err = errors.Wrap(err, "error parsing long string")
	} else {
		sop.newHandle(longStr)
	}

	return
}

func parseNull(_ *SerializedObjectParser) (interface{}, error) {
	return nil, nil
}

type endBlockT string

const endBlock endBlockT = "endBlock"

func parseEndBlockData(_ *SerializedObjectParser) (interface{}, error) {
	return endBlock, nil
}

// values reads primitive field values.
func (sop *SerializedObjectParser) values(cls *clazz) (vals map[string]interface{}, err error) {
	var exists bool

	var handler primitiveHandler

	vals = make(map[string]interface{})

	for _, field := range cls.fields {
		if field == nil {
			continue
		}

		if handler, exists = primitiveHandlers[field.typeName]; !exists {
			err = errors.Errorf("unknown field type '%s'", field.typeName)

			return
		}

		if vals[field.name], err = handler(sop); err != nil {
			err = errors.Wrap(err, "error reading primitive field value")

			return
		}
	}

	return
}

// annotationsAsMap reads values (when isBlock is false) and merges annotations then calls any relevant post processor.
func (sop *SerializedObjectParser) annotationsAsMap(cls *clazz, isBlock bool) (data map[string]interface{}, err error) {
	if isBlock {
		data = make(map[string]interface{})
	} else if data, err = sop.values(cls); err != nil {
		err = errors.Wrap(err, "error reading class data field values")

		return
	}

	var anns []interface{}

	if anns, err = sop.annotations(nil); err != nil {
		err = errors.Wrap(err, "error reading annotations")

		return
	}

	data["@"] = anns

	if !isBlock {
		if postproc, exists := KnownPostProcs[cls.name+"@"+cls.serialVersionUID]; exists {
			data, err = postproc(data, anns)
		}
	}

	return
}

// classData reads a serialized class into a generic data structure.
func (sop *SerializedObjectParser) classData(cls *clazz) (data map[string]interface{}, err error) {
	if cls == nil {
		return nil, errors.New("invalid class definition: nil")
	}

	const (
		ScSerializableWithoutWriteMethod = 0x02
		ScSerializableWithWriteMethod    = 0x03
		ScExternalizeWithBlockData       = 0x04
		ScExternalizeWithoutBlockData    = 0x0c
	)

	switch cls.flags & 0x0f {
	case ScSerializableWithoutWriteMethod: // SC_SERIALIZABLE without SC_WRITE_METHOD
		return sop.values(cls)

	case ScSerializableWithWriteMethod: // SC_SERIALIZABLE with SC_WRITE_METHOD
		return sop.annotationsAsMap(cls, false)

	case ScExternalizeWithBlockData: // SC_EXTERNALIZABLE without SC_BLOCKDATA
		return nil, errors.New("unable to parse version 1 external content")

	case ScExternalizeWithoutBlockData: // SC_EXTERNALIZABLE with SC_BLOCKDATA
		return sop.annotationsAsMap(cls, true)

	default:
		return nil, errors.Errorf("unable to deserialize class with flags %#x", cls.flags)
	}
}

// recursiveClassData recursively reads inheritance tree until it reaches java.lang.object.
func (sop *SerializedObjectParser) recursiveClassData(cls *clazz, obj map[string]interface{},
	seen map[*clazz]bool) error {
	if cls == nil {
		return nil
	}

	seen[cls] = true

	if cls.super != nil && !seen[cls.super] {
		seen[cls.super] = true
		if err := sop.recursiveClassData(cls.super, obj, seen); err != nil {
			return err
		}
	}

	extends, isMap := obj["extends"].(map[string]interface{})
	if !isMap {
		return errors.New("unexpected extends value")
	}

	fields, err := sop.classData(cls)
	if err != nil {
		return errors.Wrap(err, "error reading recursive class data")
	}

	extends[cls.name] = fields

	for name, val := range fields {
		obj[name] = val
	}

	return nil
}

func parseObject(sop *SerializedObjectParser) (obj interface{}, err error) {
	var cls *clazz

	if cls, err = sop.classDesc(); err != nil {
		err = errors.Wrap(err, "error reading object class")

		return
	}

	objMap := map[string]interface{}{
		"class":   cls,
		"extends": make(map[string]interface{}),
	}

	deferredHandle := sop.newDeferredHandle()

	seen := map[*clazz]bool{}
	if err = sop.recursiveClassData(cls, objMap, seen); err != nil {
		err = errors.Wrap(err, "error reading recursive class data")

		return
	}

	obj = deferredHandle(objMap)

	return
}

// postProcSize reads the object size as an int32 from the first data element.
func postProcSize(data []interface{}, offset int) (size int, err error) {
	if len(data) < 1 {
		err = errors.New("invalid data: at least one element required")

		return
	}

	b, isByteSlice := data[0].([]byte)
	if !isByteSlice {
		err = errors.New("unexpected data at position 0")

		return
	}

	const minLength = 4
	if len(b) < offset+minLength {
		err = errors.Errorf("incorrect data at position 0: wanted at least %d bytes, got %d", offset+minLength, len(b))

		return
	}

	var size32 int32
	if err = binary.Read(bytes.NewReader(b[offset:]), binary.BigEndian, &size32); err != nil {
		err = errors.Wrap(err, "error reading size")

		return
	}

	size = int(size32)

	return
}

// listPostProc populates the object value with a []interface{}.
func listPostProc(fields map[string]interface{}, data []interface{}) (map[string]interface{}, error) {
	size, err := postProcSize(data, 0)
	if err != nil {
		return nil, err
	}

	if len(data) != size+1 {
		return nil, errors.Errorf("incorrect number of elements: want %d got %d", size, len(data)-1)
	}

	if size > 1 {
		fields["value"] = data[1:size]
	} else {
		fields["value"] = make([]interface{}, 0)
	}

	return fields, err
}

// mapPostProc populates the object value with a map of key/value pairs.
func mapPostProc(fields map[string]interface{}, data []interface{}) (map[string]interface{}, error) {
	size, err := postProcSize(data, 4)
	if err != nil {
		return nil, err
	}

	if size*2+1 > len(data) {
		return nil, errors.Errorf("incorrect number of elements: want %d got %d", size, len(data)-1)
	}

	m := make(map[string]interface{})

	for i := 0; i < size; i++ {
		key := data[2*i+1]
		value := data[2*i+2]

		if s, isString := key.(string); isString {
			m[s] = value
		}
	}

	fields["value"] = m

	return fields, nil
}

// enumMapPostProc populates the object value with a map of key/value pairs where keys are enum constants.
func enumMapPostProc(fields map[string]interface{}, data []interface{}) (map[string]interface{}, error) {
	size, err := postProcSize(data, 0)
	if err != nil {
		return nil, err
	}

	if size*2+1 > len(data) {
		return nil, errors.Errorf("incorrect number of elements: want %d got %d", size, len(data)-1)
	}

	m := make(map[string]interface{})

	for i := 0; i < size; i++ {
		key := data[2*i+1]
		value := data[2*i+2]

		if mk, isMap := key.(map[string]interface{}); isMap {
			if s, isString := mk["value"].(string); isString {
				m[s] = value
			}
		}
	}

	fields["value"] = m

	return fields, nil
}

// hashSetPostProc populates the object value with a map of key/value pairs.
func hashSetPostProc(fields map[string]interface{}, data []interface{}) (map[string]interface{}, error) {
	size, err := postProcSize(data, 8)
	if err != nil {
		return nil, err
	}

	if len(data) != size+1 {
		return nil, errors.Errorf("incorrect number of elements: want %d got %d", size, len(data)-1)
	}

	m := make(map[string]bool)

	if size > 1 {
		for idx := range data[1:size] {
			key := data[idx+1]
			if s, isString := key.(string); isString {
				m[s] = true
			}
		}
	}

	fields["value"] = m

	return fields, nil
}

// datePostProc populates the object value with a time.Time.
func datePostProc(fields map[string]interface{}, data []interface{}) (map[string]interface{}, error) {
	if len(data) < 1 {
		return nil, errors.New("invalid data: at least one element required")
	}

	b, isByteSlice := data[0].([]byte)
	if !isByteSlice {
		return nil, errors.New("unexpected data at position 0")
	}

	const timestampBlockSize = 8
	if len(b) < timestampBlockSize {
		return nil, errors.Errorf("incorrect data at position 0: wanted 8 bytes, got %d", len(b))
	}

	var timestamp int64
	if err := binary.Read(bytes.NewReader(b[0:timestampBlockSize]), binary.BigEndian, &timestamp); err != nil {
		return nil, errors.Wrap(err, "error reading timestamp")
	}

	fields["value"] = time.Unix(0, timestamp*int64(time.Millisecond))

	return fields, nil
}
