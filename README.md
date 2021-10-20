# Java Deserialization for Go
A Go port of [nodeJavaDeserialization](https://github.com/gagern/nodeJavaDeserialization).

Much like the original, we:
> ... make no claims of completeness or correctness.
> But if you need to deserialize some Java objects using [Go],
> then you might prefer building on this over starting from scratch.


## Usage
```go
objects, err := jserial.ParseSerializedObjectMinimal(buf) 
if err != nil {
    log.Fatalf("%+v", err)
}

jsonStr, err := json.MarshalIndent(objects, "", "    ")
if err != nil {
    log.Fatalf("%+v", err)
}

fmt.Println(string(jsonStr))
```
## Usage with io.Reader
```go
sop := jserial.NewSerializedObjectParser(reader)
objects, err := sop.ParseSerializedObjectMinimal() 
if err != nil {
    log.Fatalf("%+v", err)
}

jsonStr, err := json.MarshalIndent(objects, "", "    ")
if err != nil {
    log.Fatalf("%+v", err)
}

fmt.Println(string(jsonStr))
```

Most of the time you will likely want to use `ParseSerializedObjectMinimal` which returns a simplified / JSON-like 
object representation. However, `ParseSerializedObject` is available if you need to inspect the detailed class info. 

If you do need the detailed class info you can use `ParseSerializedObject` directly which more closely matches 
nodeJavaDeserialization's object representation:   
> Each object in `objects` will contain the values of its "normal"
> fields as properties, and two hidden properties.
> One is called `class` and represents the class of the object,
> with `super` pointing at its parent class.
> The other is `extends` which is a map from fully qualified class names
> to the fields associated with that class.
> If one wants to inspect the private field of some specific class,
> using `extends` will help in cases where a more derived class contains
> another field of the same name.
> The names `class` and `extends` were deliberately chosen in such a way
> that they are keywords in Java and won't occur in normal field names.


## Custom deserialization code
If the class contained custom serialization code, the output from that is collected in a special property called `@`.
One can write post-processing code to reformat the data from that list. Such code has already been added for the 
following types:

* **`java.util.ArrayList`** - sets a `value` field which is a Go `[]interface{}`
* **`java.util.ArrayDeque`** – sets a `value` field which is a Go slice `[]interface{}`
* **`java.util.Hashtable`** – sets a `value` field which is a Go `map[string]interface{}`
* **`java.util.HashMap`** – sets a `value` field which is a Go `map[string]interface{}`
* **`java.util.EnumMap`** – sets a `value` field which is a Go `map[string]interface{}` with enum constant names as keys
* **`java.util.HashSet`** – sets a `value` field which is a Go `map[string]bool`
* **`java.util.Date`** – sets a `value` field which is a Go `time.Time`


## Fuzzing
* `cd $GOPATH/src`
* `go get -u github.com/dvyukov/go-fuzz/...`
* `go-fuzz-build github.com/jkeys089/jserial`
* `go-fuzz -bin=jserial-fuzz.zip -workdir=github.com/jkeys089/jserial/fuzzdata`


## Contributing
Bug reports, suggestions, code contributions and the likes should go to the project's GitHub page.