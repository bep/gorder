package main

import (
	"flag"
	"fmt"
	"go/token"
	"io"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
)

var (
	write = flag.Bool("w", false, "write result to (source) file instead of stdout")
)

func main() {
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 1 {
		log.Fatal("missing filename")
	}

	filename := flag.Arg(0)

	if err := handleFile(filename, *write); err != nil {
		log.Fatal(err)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: gorder [flags] [filename]\n")
	flag.PrintDefaults()
}

func handleFile(filename string, write bool) error {
	var perm os.FileMode = 0644

	f, err := os.Open(filename)
	if err != nil {
		return err
	}

	fi, err := f.Stat()
	if err != nil {
		return err
	}

	perm = fi.Mode().Perm()

	src, err := ioutil.ReadAll(f)
	if err != nil {
		return err
	}

	f.Close()

	file, err := decorator.Parse(src)
	if err != nil {
		return err
	}

	dst.Inspect(file, func(n dst.Node) bool {
		switch v := n.(type) {
		case *dst.File:
			sortDecls(v.Decls)
		case *dst.InterfaceType:
			sortFieldList(v.Methods)
		case *dst.StructType:
		case *dst.FieldList:

		case nil:
		default:

		}

		return true

	})

	var out io.Writer

	if write {
		f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, perm)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	} else {
		out = os.Stdout
	}

	//	out = ioutil.Discard

	if err := decorator.Fprint(out, file); err != nil {
		log.Fatal(err)
	}

	return nil
}

func sortFieldList(fields *dst.FieldList) {
	sort.SliceStable(fields.List, func(i, j int) bool {
		fi, fj := fields.List[i], fields.List[j]
		ni, nj := len(fi.Names), len(fj.Names)
		if ni == 0 && nj == 0 {
			return less(fi.Type, fj.Type)
		}

		if ni == 0 {
			return true
		}

		if nj == 0 {
			return false
		}

		return lessStringers(fi.Names[0], fj.Names[0])
	})
}

func sortDecls(decls []dst.Decl) {
	sort.SliceStable(decls, func(i, j int) bool {
		di, dj := decls[i], decls[j]

		if preserveOrder(di) || preserveOrder(dj) {
			return i < j
		}

		if isFuncDecl(di) && !isFuncDecl(dj) {
			return false
		}

		if isFuncDecl(di) != isFuncDecl(dj) {
			return true
		}

		if isFuncDecl(di) {
			f1, f2 := di.(*dst.FuncDecl), dj.(*dst.FuncDecl)

			if f1.Recv == nil && f2.Recv != nil {
				// Sort functions before methods
				return true
			}

			f1r, f2r := fieldListName(f1.Recv), fieldListName(f2.Recv)

			if f1r != f2r {
				return f1r < f2r
			}

			return lessStringers(f1.Name, f2.Name)
		}

		m1, m2 := di.(*dst.GenDecl), dj.(*dst.GenDecl)

		if m1.Tok == m2.Tok {
			if m1.Tok == token.TYPE {
				return lessStringers(m1.Specs[0].(*dst.TypeSpec).Name, m2.Specs[0].(*dst.TypeSpec).Name)
			}
		}

		return i < j
	})
}

func fieldListName(list *dst.FieldList) string {
	if list == nil {
		return ""
	}
	var b strings.Builder
	for _, v := range list.List {
		switch xv := v.Type.(type) {
		case *dst.StarExpr:
			if si, ok := xv.X.(*dst.Ident); ok {
				b.WriteString(si.Name)
			}
		case *dst.Ident:
			b.WriteString(xv.Name)
		}
	}

	return b.String()
}

func less(s, t interface{}) bool {
	strf := func(in interface{}) string {
		switch v := in.(type) {
		case *dst.SelectorExpr:
			return fmt.Sprintf("%s.%s", v.X, v.Sel)
		case *dst.Ident:
			return v.String()
		default:
			panic(fmt.Sprintf("type %T", in))
		}
	}

	return lesss(strf(s), strf(t))

}

func lessStringers(s1, s2 fmt.Stringer) bool {
	return lesss(s1.String(), s2.String())
}

func lesss(s1, s2 string) bool {
	e1, e2 := isExported(s1), isExported(s2)
	if e1 != e2 {
		if e1 {
			return true
		} else {
			return false
		}
	}

	return s1 < s2

}

func preserveOrder(decl dst.Decl) bool {
	switch v := decl.(type) {
	case *dst.GenDecl:
		return v.Tok == token.PACKAGE || v.Tok == token.IMPORT
	default:
		return false
	}
}

func isFuncDecl(decl dst.Decl) bool {
	switch decl.(type) {
	case *dst.FuncDecl:
		return true
	default:
		return false
	}
}

func isExported(name string) bool {
	if strings.Contains(name, ".") {
		return true
	}
	for _, r := range name {
		return unicode.IsUpper(r)
	}
	return false
}