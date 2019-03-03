package jserial

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	initSerializedObjects()
	os.Exit(m.Run())
}

// -------------------------------- //
// -- Begin Negative Tests Cases -- //
// -------------------------------- //

const (
	streamMagic      = "aced"
	streamVersion    = "0005"
	tcNull           = "70"
	tcReference      = "71"
	tcClassDesc      = "72"
	tcObject         = "73"
	tcString         = "74"
	tcArray          = "75"
	tcClass          = "76"
	tcBlockData      = "77"
	tcEndBlockData   = "78"
	tcReset          = "79"
	tcBlockDataLong  = "7a"
	tcException      = "7b"
	tcLongString     = "7c"
	tcProxyClassDesc = "7d"
	tcEnum           = "7e"
	baseWireHandle   = "7e0000"
	scWriteMethod    = "01"
	scBlockData      = "08"
	scSerializable   = "02"
	scExternalizable = "04"
	scEnum           = "10"

	streamPrefix = streamMagic + streamVersion + tcObject
	serialVer    = "1234567887654321"
)

var (
	strValAsHex  = hex.EncodeToString([]byte("abcdefg"))
	someClassEnc = encodeStr("SomeClass")
	fooEnc       = encodeStr("foo")
)

func encodeStr(s string) string {
	hexStr := hex.EncodeToString([]byte(s))
	return fmt.Sprintf("%04x%s", uint16(len(hexStr)>>1), hexStr)
}

func streamHex(overrideName, overrideVal string) string {

	vals := map[string]string{
		"flags":     scSerializable,
		"fieldType": hex.EncodeToString([]byte("I")),
		"classDesc": tcClassDesc,
	}

	if overrideName == "fieldType" {
		vals["fieldType"] = hex.EncodeToString([]byte(overrideVal))
	} else {
		vals[overrideName] = overrideVal
	}

	return streamPrefix + vals["classDesc"] + someClassEnc + serialVer + vals["flags"] + "0001" + vals["fieldType"] +
		fooEnc + tcEndBlockData + tcNull + "01234567"

}

func getErr(hexStr string) (err error) {
	var b []byte
	if b, err = hex.DecodeString(hexStr); err != nil {
		return
	}
	_, err = ParseSerializedObjectMinimal(b)
	return
}

func TestBadMagicValue(t *testing.T) {
	err := getErr("acde0005")
	if err == nil || !strings.Contains(err.Error(), "STREAM_MAGIC") {
		t.Fail()
	}
}

func TestBadVersion(t *testing.T) {
	err := getErr("aced0004")
	if err == nil || !strings.Contains(err.Error(), "protocol version") {
		t.Fail()
	}
}

func TestStringTooLong(t *testing.T) {
	err := getErr(streamMagic + streamVersion + tcLongString + "7000000000000000" + strValAsHex)
	if err == nil || !strings.Contains(err.Error(), "string larger than") {
		t.Fail()
	}
}

func TestStringPrematureEnd(t *testing.T) {
	expectedErrStr := "premature end"
	err := getErr(streamMagic + streamVersion + tcString + "0008" + strValAsHex)
	if err == nil || !strings.Contains(err.Error(), expectedErrStr) {
		t.Fail()
	}
	err = getErr(streamMagic + streamVersion + tcString + "00" + strValAsHex)
	if err == nil || !strings.Contains(err.Error(), expectedErrStr) {
		t.Fail()
	}
}

func TestResetNotSupported(t *testing.T) {
	err := getErr(streamMagic + streamVersion + tcReset)
	if err == nil || !strings.Contains(err.Error(), "parsing Reset") {
		t.Fail()
	}
}

func TestExceptionNotSupported(t *testing.T) {
	err := getErr(streamMagic + streamVersion + tcException)
	if err == nil || !strings.Contains(err.Error(), "parsing Exception") {
		t.Fail()
	}
}

func TestProxyClassDescNotSupported(t *testing.T) {
	err := getErr(streamMagic + streamVersion + tcProxyClassDesc)
	if err == nil || !strings.Contains(err.Error(), "parsing ProxyClassDesc") {
		t.Fail()
	}
}

func TestUnkownType(t *testing.T) {
	err := getErr(streamMagic + streamVersion + "67")
	if err == nil || !strings.Contains(err.Error(), "unknown type 0x67") {
		t.Fail()
	}
}

func TestBadFlags(t *testing.T) {
	err := getErr(streamHex("flags", "00"))
	if err == nil || !strings.Contains(err.Error(), "flags 0x0") {
		t.Fail()
	}
}

func TestV1Extern(t *testing.T) {
	err := getErr(streamHex("flags", scExternalizable))
	if err == nil || !strings.Contains(err.Error(), "version 1 external") {
		t.Fail()
	}
}

func TestUnkownPrimitive(t *testing.T) {
	err := getErr(streamHex("fieldType", "Q"))
	if err == nil || !strings.Contains(err.Error(), "field type 'Q'") {
		t.Fail()
	}
}

func TestBadClassDesc(t *testing.T) {
	err := getErr(streamHex("classDesc", tcObject))
	if err == nil || !strings.Contains(err.Error(), "Object not allowed") {
		t.Fail()
	}
}

func TestWrongHashSetSize(t *testing.T) {
	hexStr := streamPrefix + tcClassDesc + encodeStr("java.util.HashSet") + "ba44859596b8b734" + "03" + "0000" +
		tcEndBlockData + tcNull + tcBlockData + "0c" + "00000003" + "00000000" + "00000003" + tcString + fooEnc +
		tcEndBlockData
	err := getErr(hexStr)
	if err == nil || !strings.Contains(err.Error(), "incorrect number of elements") {
		t.Fail()
	}
}

// -------------------------------- //
// -- Begin Positive Tests Cases -- //
// -------------------------------- //

var objs map[string][]byte

func initSerializedObjects() {

	objs = make(map[string][]byte)

	f := func(k, v string) {
		b, _ := base64.StdEncoding.DecodeString(v)
		if strings.HasPrefix(v, "H4sI") {
			r := bytes.NewReader(b)
			gr, _ := gzip.NewReader(r)
			b, _ = ioutil.ReadAll(gr)
		}
		objs[k] = b
	}

	f(`canary`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABdXEAfgAAAAAAAnEAfgADdAADRW5k`)
	f(`string`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABdAAIc29tZXRleHR1cQB+AAAAAAACcQB+AAR0AANFbmQ=`)
	f(`longStr`, `H4sIAAAAAAAAAO3JuwnCABRA0Wc0veAUNlnATrATbAWr+CEYQvCTiIVkBjdwAWdxE3eQgGOcU12472+k7SUmm2WZ3/KsyusiW23Lw66ZPT/r1/g6rZKI+ykikibS+aE41ufoYvCIXv8AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA/tpzdNFL+hg1MVzU+x8AC//OVwACAA==`)
	f(`null`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABcHVxAH4AAAAAAAJxAH4AA3QAA0VuZA==`)
	f(`dupe`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABc3IAEkJhc2VDbGFzc1dpdGhGaWVsZAAAAAAAABI0AgABSQADZm9veHAAAAB7dAAFZGVsaW1xAH4ABHVxAH4AAAAAAAJxAH4ABnQAA0VuZA==`)
	f(`prim`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABc3IAD1ByaW1pdGl2ZUZpZWxkcwAAEjRWeJq8AgAIWgACYm9CAAJieUMAAWNEAAFkRgABZkkAAWlKAAFsUwABc3hwAesSNEAorhR64UeuQpkAAP///4X////////86/44dXEAfgAAAAAAAnEAfgAFdAADRW5k`)
	f(`boxedPrim`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABc3IAEWphdmEubGFuZy5JbnRlZ2VyEuKgpPeBhzgCAAFJAAV2YWx1ZXhyABBqYXZhLmxhbmcuTnVtYmVyhqyVHQuU4IsCAAB4cP///4VzcgAPamF2YS5sYW5nLlNob3J0aE03EzRg2lICAAFTAAV2YWx1ZXhxAH4ABP44c3IADmphdmEubGFuZy5Mb25nO4vkkMyPI98CAAFKAAV2YWx1ZXhxAH4ABP////////zrc3IADmphdmEubGFuZy5CeXRlnE5ghO5Q9RwCAAFCAAV2YWx1ZXhxAH4ABOtzcgAQamF2YS5sYW5nLkRvdWJsZYCzwkopa/sEAgABRAAFdmFsdWV4cQB+AARAKK4UeuFHrnNyAA9qYXZhLmxhbmcuRmxvYXTa7cmi2zzw7AIAAUYABXZhbHVleHEAfgAEQpkAAHNyABFqYXZhLmxhbmcuQm9vbGVhbs0gcoDVnPruAgABWgAFdmFsdWV4cAFzcgATamF2YS5sYW5nLkNoYXJhY3RlcjSLR9lrGiZ4AgABQwAFdmFsdWV4cBI0dXEAfgAAAAAAAnEAfgAUdAADRW5k`)
	f(`inherited`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABc3IAHERlcml2ZWRDbGFzc1dpdGhBbm90aGVyRmllbGQAAAAAAAAjRQIAAUkAA2JhcnhyABJCYXNlQ2xhc3NXaXRoRmllbGQAAAAAAAASNAIAAUkAA2Zvb3hwAAAAewAAAOp1cQB+AAAAAAACcQB+AAZ0AANFbmQ=`)
	f(`dupeField`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABc3IAGURlcml2ZWRDbGFzc1dpdGhTYW1lRmllbGQAAAAAAAA0VgIAAUkAA2Zvb3hyABJCYXNlQ2xhc3NXaXRoRmllbGQAAAAAAAASNAIAAUkAA2Zvb3hwAAAAewAAAVl1cQB+AAAAAAACcQB+AAZ0AANFbmQ=`)
	f(`primArray`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABdXIAAltJTbpgJnbqsqUCAAB4cAAAAAMAAAAMAAAAIgAAADh1cQB+AAAAAAACcQB+AAV0AANFbmQ=`)
	f(`nestedArr`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABdXIAFFtbTGphdmEubGFuZy5TdHJpbmc7Mk0JrYQy5FcCAAB4cAAAAAJ1cgATW0xqYXZhLmxhbmcuU3RyaW5nO63SVufpHXtHAgAAeHAAAAACdAABYXQAAWJ1cQB+AAUAAAABdAABY3VxAH4AAAAAAAJxAH4AC3QAA0VuZA==`)
	f(`arrFields`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABc3IAC0FycmF5RmllbGRzAAAAAAAAAAECAANbAAJpYXQAAltJWwADaWFhdAADW1tJWwACc2F0ABNbTGphdmEvbGFuZy9TdHJpbmc7eHB1cgACW0lNumAmduqypQIAAHhwAAAAAwAAAAwAAAAiAAAAOHVyAANbW0kX9+RPGY+JPAIAAHhwAAAAAnVxAH4ACAAAAAIAAAALAAAADHVxAH4ACAAAAAMAAAAVAAAAFgAAABd1cgATW0xqYXZhLmxhbmcuU3RyaW5nO63SVufpHXtHAgAAeHAAAAACdAADZm9vdAADYmFydXEAfgAAAAAAAnEAfgASdAADRW5k`)
	f(`enum`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABfnIACFNvbWVFbnVtAAAAAAAAAAASAAB4cgAOamF2YS5sYW5nLkVudW0AAAAAAAAAABIAAHhwdAADT05FfnEAfgADdAAFVEhSRUVxAH4AB3VxAH4AAAAAAAJxAH4ACXQAA0VuZA==`)
	f(`exception`, `H4sIAAAAAAAAAIVSXWvUQBS9m83WNqAufmGrriBaUWQXQYWSIthllWKsoBWEBWU2ud1OnUzizMSNCqKIr+KrgvoHfBX8AX48FEQEH330TZ99seDc1P2Qgs7DTHJz7plzTu6rH1DJFGxvByvsFqsLJrv1i50VDI3/5OPVl1V9RDgAeQoAjoHKHHa5vAn3oKQVTA1bLmXS8BhbeYip4Yl8cX3snHfi4TfqtexD4ADxaW3/6Sl/79sNiMVllfRYR+CX9ycPz/TerJbBDaASskyjgZ2FzgYhGwOkH8DmCA3j4gJqzboWt20Ed9koLrt+GzxtWHhjUbHQImrtvyD9Dy2BMUpjKXfoLE2VJcRooFobqK63ZYaLRsC18fOUAvEMjJ1nnSSJbZy10Tg3cjtnj87Orh2Y6SdLYe75RwObePds+tTXRw648+AJLnEhizuoAtgSYSgY2WsKpjUJ2RTA+BIXuMBi/PPuxWiWk2hQKS69a6DcFNbQuN3rdLstWKDVMl1oIYf1ZiKEHQayfvCKjJOIL3GKnJz/2nro+Ovvj6sOlAJwha0Q+4T9ncf+TzCsT87B/dVrP2sFTSk0sGsk4SHM5qz7g1Iwn1GK3SYd+YPP+55+YM/LUJoHV/M7WBiEnkt7TqJ25xkdtBx6mLReWzL6DS3112r+AgAA`)
	f(`custom`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABc3IADEN1c3RvbUZvcm1hdAAAAAAAAAABAwABSQADZm9veHAAADA5dwu16y0AtestALXrLXQACGFuZCBtb3JleHVxAH4AAAAAAAJxAH4ABnQAA0VuZA==`)
	f(`extern`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABc3IACEV4dGVybmFs8N9gtNEyHREMAAB4cHcPAAAAC7XrLQC16y0AtestdAAIYW5kIG1vcmV4dXEAfgAAAAAAAnEAfgAGdAADRW5k`)
	f(`longExtern`, `H4sIAAAAAAAAAFvzloG1tIhBONonK7EsUS8nMS9dzz8pKzW5xHrCuYj5AsWaOUwMDBUFDAwMTCUMrE6p6Zl5hQx1DIzFRQwcrhUlqUV5iTkf7idsuWgkK8gDUlkFVMkCxAwMjEzMLKxs7BycXNw8vHz8AoJCwiKiYuISklLSMrJy8gqKSsoqqmrqGppa2jq6evoGhkbGJqZm5haWVtY2tnb2Do5Ozi6ubu4enl7ePr5+/gGBQcEhoWHhEZFR0TGxcfEJiUnJKalp6RmZWdk5uXn5BYVFxSWlZeUVlVXVNbV19Q2NTc0trW3tHZ1d3T29ff0TJk6aPGXqtOkzZs6aPWfuvPkLFi5avGTpsuUrVq5avWbtuvUbNm7avGXrtu07du7avWfvvv0HDh46fOToseMnTp46febsufMXLl66fOXqtes3bt66fefuvfsPHj56/OTps+cvXr56/ebtu/cfPn76/OXrt+8/fv76/efvv/8j3f8lDByJeSkKuflFqRWloDQDAkwgBlsJA7NrXgoAi1fv5nwCAAA=`)
	f(`hashMapStr`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABc3IAEWphdmEudXRpbC5IYXNoTWFwBQfawcMWYNEDAAJGAApsb2FkRmFjdG9ySQAJdGhyZXNob2xkeHA/QAAAAAAADHcIAAAAEAAAAAJ0AANiYXJ0AANiYXp0AANmb29zcgARamF2YS5sYW5nLkludGVnZXIS4qCk94GHOAIAAUkABXZhbHVleHIAEGphdmEubGFuZy5OdW1iZXKGrJUdC5TgiwIAAHhwAAAAe3h1cQB+AAAAAAACcQB+AAt0AANFbmQ=`)
	f(`hashMapObj`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABc3IAEWphdmEudXRpbC5IYXNoTWFwBQfawcMWYNEDAAJGAApsb2FkRmFjdG9ySQAJdGhyZXNob2xkeHA/QAAAAAAADHcIAAAAEAAAAAJ0AANiYXp0AANiYXJzcgARamF2YS5sYW5nLkludGVnZXIS4qCk94GHOAIAAUkABXZhbHVleHIAEGphdmEubGFuZy5OdW1iZXKGrJUdC5TgiwIAAHhwAAAAe3QAA2Zvb3hxAH4ACXVxAH4AAAAAAAJxAH4AC3QAA0VuZA==`)
	f(`hashMapEmpty`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABc3IAEWphdmEudXRpbC5IYXNoTWFwBQfawcMWYNEDAAJGAApsb2FkRmFjdG9ySQAJdGhyZXNob2xkeHA/QAAAAAAAAHcIAAAAEAAAAAB4dXEAfgAAAAAAAnEAfgAFdAADRW5k`)
	f(`hashTblStr`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABc3IAE2phdmEudXRpbC5IYXNodGFibGUTuw8lIUrkuAMAAkYACmxvYWRGYWN0b3JJAAl0aHJlc2hvbGR4cD9AAAAAAAAIdwgAAAALAAAAAnQAA2JhcnQAA2JhenQAA2Zvb3NyABFqYXZhLmxhbmcuSW50ZWdlchLioKT3gYc4AgABSQAFdmFsdWV4cgAQamF2YS5sYW5nLk51bWJlcoaslR0LlOCLAgAAeHAAAAB7eHVxAH4AAAAAAAJxAH4AC3QAA0VuZA==`)
	f(`enumMap`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABc3IAEWphdmEudXRpbC5FbnVtTWFwBl19976QfKEDAAFMAAdrZXlUeXBldAARTGphdmEvbGFuZy9DbGFzczt4cHZyAAhTb21lRW51bQAAAAAAAAAAEgAAeHIADmphdmEubGFuZy5FbnVtAAAAAAAAAAASAAB4cHcEAAAAAn5xAH4ABnQAA09ORXNyABFqYXZhLmxhbmcuSW50ZWdlchLioKT3gYc4AgABSQAFdmFsdWV4cgAQamF2YS5sYW5nLk51bWJlcoaslR0LlOCLAgAAeHAAAAB7fnEAfgAGdAAFVEhSRUV0AANiYXp4cQB+AAlxAH4ADnVxAH4AAAAAAAJxAH4AEXQAA0VuZA==`)
	f(`arrayList`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABc3IAE2phdmEudXRpbC5BcnJheUxpc3R4gdIdmcdhnQMAAUkABHNpemV4cAAAAAJ3BAAAAAJ0AANmb29zcgARamF2YS5sYW5nLkludGVnZXIS4qCk94GHOAIAAUkABXZhbHVleHIAEGphdmEubGFuZy5OdW1iZXKGrJUdC5TgiwIAAHhwAAAAe3h1cQB+AAAAAAACcQB+AAl0AANFbmQ=`)
	f(`arrayDeque`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABc3IAFGphdmEudXRpbC5BcnJheURlcXVlIHzaLiQNoIsDAAB4cHcEAAAAAnQAA2Zvb3NyABFqYXZhLmxhbmcuSW50ZWdlchLioKT3gYc4AgABSQAFdmFsdWV4cgAQamF2YS5sYW5nLk51bWJlcoaslR0LlOCLAgAAeHAAAAB7eHVxAH4AAAAAAAJxAH4ACXQAA0VuZA==`)
	f(`hashSet`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABc3IAEWphdmEudXRpbC5IYXNoU2V0ukSFlZa4tzQDAAB4cHcMAAAAED9AAAAAAAACdAADZm9vc3IAEWphdmEubGFuZy5JbnRlZ2VyEuKgpPeBhzgCAAFJAAV2YWx1ZXhyABBqYXZhLmxhbmcuTnVtYmVyhqyVHQuU4IsCAAB4cAAAAHt4dXEAfgAAAAAAAnEAfgAJdAADRW5k`)
	f(`date`, `rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAJ0AAVCZWdpbnEAfgABc3IADmphdmEudXRpbC5EYXRlaGqBAUtZdBkDAAB4cHcIAAAAXgkZ7aB4dXEAfgAAAAAAAnEAfgAFdAADRW5k`)

}

func prettyPrint(t *testing.T, deserializedContent []interface{}) {
	jsonStr, err := json.MarshalIndent(deserializedContent, "", "    ")
	if err != nil {
		t.Fatalf("failed to pretty print deserialized content JSON: %+v", err)
	}
	fmt.Println(string(jsonStr))
}

func TestDeserialize(t *testing.T) {
	if _, err := ParseSerializedObjectMinimal(objs["canary"]); err != nil {
		t.Fail()
	}
}

func TestDeserializeString(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["string"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	s, isString := obj[1].(string)
	if !isString || s != "sometext" {
		t.Fail()
	}
}

func TestDeserializeLongString(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["longStr"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	s, isString := obj[1].(string)
	if !isString || s != strings.Repeat("x", 131072) {
		t.Fail()
	}
}

func TestDeserializeNull(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["null"])
	if err != nil || len(obj) != 3 || obj[1] != nil {
		t.Fail()
	}
}

func TestDeserializeDuplicate(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["dupe"])
	if err != nil || len(obj) != 5 {
		t.Fail()
	}
	if !reflect.DeepEqual(obj[1], obj[3]) {
		t.Fail()
	}
}

func TestDeserializePrimitives(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["prim"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	m, isMap := obj[1].(map[string]interface{})
	if !isMap {
		t.Fail()
	}
	expected := map[string]interface{}{
		"i":  int32(-123),
		"s":  int16(-456),
		"l":  int64(-789),
		"by": int8(-21),
		"d":  float64(12.34),
		"f":  float32(76.5),
		"bo": true,
		"c":  "ሴ",
	}
	for k, v := range expected {
		if m[k] != v {
			t.Fail()
		}
	}
}

func TestDeserializeBoxedPrimitives(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["boxedPrim"])
	if err != nil || len(obj) != 10 {
		t.Fail()
	}
	expected := []interface{}{
		int32(-123),
		int16(-456),
		int64(-789),
		int8(-21),
		float64(12.34),
		float32(76.5),
		true,
		"ሴ",
	}
	for idx, v := range expected {
		if obj[idx+1] != v {
			t.Fail()
		}
	}
}

func TestDeserializeInherited(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["inherited"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	m, isMap := obj[1].(map[string]interface{})
	if !isMap {
		t.Fail()
	}
	if m["bar"] != int32(234) {
		t.Fail()
	}
	if m["foo"] != int32(123) {
		t.Fail()
	}
}

func TestDeserializeDuplicateField(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["dupeField"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	m, isMap := obj[1].(map[string]interface{})
	if !isMap {
		t.Fail()
	}
	if m["foo"] != int32(345) {
		t.Fail()
	}
}

func TestDeserializePrimitiveArray(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["primArray"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	arr, isArray := obj[1].([]interface{})
	if !isArray {
		t.Fail()
	}
	expected := []interface{}{
		int32(12),
		int32(34),
		int32(56),
	}
	for idx, v := range expected {
		if arr[idx] != v {
			t.Fail()
		}
	}
}

func TestDeserializeNestedArray(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["nestedArr"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	arr, isArray := obj[1].([]interface{})
	if !isArray {
		t.Fail()
	}
	expected := []interface{}{
		[]interface{}{"a", "b"},
		[]interface{}{"c"},
	}
	for idx, v := range expected {
		if !reflect.DeepEqual(arr[idx], v) {
			t.Fail()
		}
	}
}

func TestDeserializeArrayFields(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["arrFields"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	m, isMap := obj[1].(map[string]interface{})
	if !isMap {
		t.Fail()
	}
	expected := map[string]interface{}{
		"ia":  []interface{}{int32(12), int32(34), int32(56)},
		"iaa": []interface{}{[]interface{}{int32(11), int32(12)}, []interface{}{int32(21), int32(22), int32(23)}},
		"sa":  []interface{}{"foo", "bar"},
	}
	for k, v := range expected {
		if !reflect.DeepEqual(m[k], v) {
			t.Fail()
		}
	}
}

func TestDeserializeEnum(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["enum"])
	if err != nil || len(obj) != 5 {
		t.Fail()
	}
	expected := []interface{}{
		"ONE",
		"THREE",
		"THREE",
	}
	for idx, v := range expected {
		if obj[idx+1] != v {
			t.Fail()
		}
	}
}

func TestDeserializeException(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["exception"])
	//prettyPrint(t, obj)
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	m, isMap := obj[1].(map[string]interface{})
	if !isMap {
		t.Fail()
	}
	if m["detailMessage"] != "Kaboom" {
		t.Fail()
	}
}

func TestDeserializeCustom(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["custom"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	m, isMap := obj[1].(map[string]interface{})
	if !isMap {
		t.Fail()
	}
	if m["foo"] != int32(12345) {
		t.Fail()
	}
	arr, isArray := m["@"].([]interface{})
	if !isArray || len(arr) != 2 {
		t.Fail()
	}
	a0, isBytes := arr[0].([]byte)
	if !isBytes {
		t.Fail()
	}
	a0Expected, decodeErr := hex.DecodeString("b5eb2d00b5eb2d00b5eb2d")
	if decodeErr != nil || bytes.Compare(a0, a0Expected) != 0 {
		t.Fail()
	}
	if arr[1] != "and more" {
		t.Fail()
	}
}

func TestDeserializeExtern(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["extern"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	m, isMap := obj[1].(map[string]interface{})
	if !isMap {
		t.Fail()
	}
	arr, isArray := m["@"].([]interface{})
	if !isArray || len(arr) != 2 {
		t.Fail()
	}
	a0, isBytes := arr[0].([]byte)
	if !isBytes {
		t.Fail()
	}
	a0Expected, decodeErr := hex.DecodeString("0000000bb5eb2d00b5eb2d00b5eb2d")
	if decodeErr != nil || bytes.Compare(a0, a0Expected) != 0 {
		t.Fail()
	}
	if arr[1] != "and more" {
		t.Fail()
	}
}

func TestDeserializeLongExtern(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["longExtern"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	m, isMap := obj[1].(map[string]interface{})
	if !isMap {
		t.Fail()
	}
	arr, isArray := m["@"].([]interface{})
	if !isArray || len(arr) != 2 {
		t.Fail()
	}
	a0, isBytes := arr[0].([]byte)
	if !isBytes || len(a0) != 516 {
		t.Fail()
	}
	if arr[1] != "and more" {
		t.Fail()
	}
}

func TestDeserializeHashMapWithStrKeys(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["hashMapStr"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	expected := map[string]interface{}{
		"bar": "baz",
		"foo": int32(123),
	}
	if !reflect.DeepEqual(obj[1], expected) {
		t.Fail()
	}
}

func TestDeserializeHashMapWithObectKeys(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["hashMapObj"])
	if err != nil || len(obj) != 4 {
		t.Fail()
	}
	expected := map[string]interface{}{
		"baz": "bar",
	}
	if !reflect.DeepEqual(obj[1], expected) {
		t.Fail()
	}
	if obj[2] != int32(123) {
		t.Fail()
	}
}

func TestDeserializeHashMapEmpty(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["hashMapEmpty"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	m, isMap := obj[1].(map[string]interface{})
	if !isMap || len(m) != 0 {
		t.Fail()
	}
}

func TestDeserializeHashTableWithStringKeys(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["hashTblStr"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	expected := map[string]interface{}{
		"bar": "baz",
		"foo": int32(123),
	}
	if !reflect.DeepEqual(obj[1], expected) {
		t.Fail()
	}
}

func TestDeserializeEnumMap(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["enumMap"])
	if err != nil || len(obj) != 5 {
		t.Fail()
	}
	expected := map[string]interface{}{
		"THREE": "baz",
		"ONE":   int32(123),
	}
	if !reflect.DeepEqual(obj[1], expected) {
		t.Fail()
	}
	if obj[2] != "ONE" {
		t.Fail()
	}
	if obj[3] != "THREE" {
		t.Fail()
	}
}

func TestDeserializeArrayList(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["arrayList"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	expected := []interface{}{"foo"}
	if !reflect.DeepEqual(obj[1], expected) {
		t.Fail()
	}
}

func TestDeserializeArrayDeque(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["arrayDeque"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	expected := []interface{}{"foo"}
	if !reflect.DeepEqual(obj[1], expected) {
		t.Fail()
	}
}

func TestDeserializeHashSet(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["hashSet"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	expected := map[string]bool{
		"foo": true,
	}
	if !reflect.DeepEqual(obj[1], expected) {
		t.Fail()
	}
}

func TestDeserializeDate(t *testing.T) {
	obj, err := ParseSerializedObjectMinimal(objs["date"])
	if err != nil || len(obj) != 3 {
		t.Fail()
	}
	if !reflect.DeepEqual(obj[1], time.Unix(403879620, 0)) {
		t.Fail()
	}
}
