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

func NewFoo() {

}

func (m myStruct) ExportedMethod() {

}

func theFunction() string {
	return "asdf"
}

func MyFunction() string {
	return "asdf"
}

type my struct {
}

func (m my) myMethod() {

}

func aFunction() string {
	return "asdf"
}

func newFoo() string {
	return "asdf"
}

type Moo interface {
	C()
	G()
	B()
}
