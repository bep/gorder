package main

import (
	"flag"
	"fmt"
	"go/token"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
)

var (
	write = flag.Bool("w", false, "write result to (source) file instead of stdout")
)

const (
	magicTypeMarker = "______"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("error: ")
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 1 {
		log.Fatal("missing filename")
	}

	filenames, err := filepath.Glob(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}

	w := *write

	if len(filenames) > 1 && !w {
		log.Fatal("multiple file matches require the -w flag")
	}

	for _, filename := range filenames {
		if err := handleFile(filename, w); err != nil {
			log.Fatal(err)
		}
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

		ll := lessStringers(fi.Names[0], fj.Names[0])

		return ll
	})
}

func sortDecls(decls []dst.Decl) {
	sort.SliceStable(decls, func(i, j int) bool {
		di, dj := decls[i], decls[j]

		const (
			// Less means higher up. We do some adjustments between these,
			// so keep some empty space.
			funcWeight            = 200
			typeWeight            = 100
			constructorFuncWeight = 50 // newSomething
			exportedFuncWeight    = 30
			mainFuncWeight        = 10
		)

		if preserveOrder(di) || preserveOrder(dj) {
			return i < j
		}

		funcName := func(d dst.Decl) (string, int) {
			f, ok := d.(*dst.FuncDecl)
			if !ok {
				return "", -1
			}

			fr := fieldListName(f.Recv)

			name := f.Name.String()

			if fr == "" {
				if name == "main" {
					return name, mainFuncWeight
				}

				if strings.HasPrefix(name, "new") {
					return name, constructorFuncWeight
				}

				if firstUpper(name) {
					weight := exportedFuncWeight
					if strings.HasPrefix(name, "New") {
						weight--
					}
					return name, weight
				}

				return name, funcWeight
			}

			// This is a method. We want that below the receiver type definition, if possible.
			return fmt.Sprintf("%s.%s", fr, name), typeWeight

		}

		genName := func(d dst.Decl) (string, int) {
			m, ok := d.(*dst.GenDecl)
			if !ok {
				return "", -1
			}

			if m.Tok == token.TYPE {
				// Return on the form receiver.____ to make sure it's grouped with the
				// methods it owns.
				return m.Specs[0].(*dst.TypeSpec).Name.String() + "." + magicTypeMarker, typeWeight
			}

			return "", -1

		}

		name := func(d dst.Decl) (string, int) {
			s, weight := funcName(d)
			if weight != -1 {
				return s, weight
			}

			return genName(d)

		}

		si, weighti := name(di)
		sj, weightj := name(dj)

		if weighti == -1 && weightj == -1 {
			return i < j
		}

		if weighti != weightj {
			return weighti < weightj
		}

		return lesss(si, sj)
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

func weightAdjustment(name string) int {
	w := 0

	if name == magicTypeMarker {
		w -= 5
	}
	// Exported funcs
	if firstUpper(name) {
		w -= 2
	}

	// Exported constructor funcs.
	if strings.HasPrefix(name, "New") {
		w--
	}

	return w
}

func lesss(s1, s2 string) bool {
	s1r, s1name := splitOnDot(s1)
	s2r, s2name := splitOnDot(s2)

	if s1r != s2r {
		// Different receiver types
		return s1r < s2r
	}

	s1w := 100
	s2w := 100

	s1w += weightAdjustment(s1name)
	s2w += weightAdjustment(s2name)

	if s1w != s2w {
		return s1w < s2w
	}

	var s1prefix, s2prefix string

	s1name, s1prefix = trimCommonPrefix(s1name)
	s2name, s2prefix = trimCommonPrefix(s2name)

	if s1prefix != "" && s2prefix != "" {
		return s1prefix < s2prefix
	}

	return s1name < s2name

}

var commonPrefixes = []string{"Is", "Has", "Get", "All", "Create", "New", "Err", "Error", "Init", "Find", "Set", "Render"}

func trimCommonPrefix(s string) (string, string) {
	for _, prefix := range commonPrefixes {
		if strings.HasPrefix(s, prefix) {
			return prefix, strings.TrimPrefix(s, prefix)
		}
		if strings.HasPrefix(s, strings.ToLower(prefix)) {
			return prefix, strings.TrimPrefix(s, strings.ToLower(prefix))
		}
	}

	return "", s

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

func splitOnDot(name string) (string, string) {
	parts := strings.Split(name, ".")
	if len(parts) > 2 {
		panic("too many")
	}
	if len(parts) == 1 {
		return "", name
	}

	return parts[0], parts[1]

}

func firstUpper(name string) bool {
	for _, r := range name {
		return unicode.IsUpper(r)
	}
	return false
}
