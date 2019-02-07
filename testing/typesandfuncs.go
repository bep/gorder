package testing

var (
	a = 1
	b = 2
)

type myString string

type myStruct struct {
}

func (m myStruct) privateMethod() {

}

func (m myStruct) ExportedMethod() {

}

func theFunction() string {
	return "asdf"
}
