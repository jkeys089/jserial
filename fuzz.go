// +build gofuzz

package jserial

// Fuzz is the go-fuzz entrypoint for fuzzing serialized object parsing
func Fuzz(data []byte) int {
	if _, err := ParseSerializedObjectMinimal(data); err != nil {
		return 0
	}
	return 1
}
