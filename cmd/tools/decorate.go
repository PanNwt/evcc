package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"

	combinations "github.com/mxschmitt/golang-combinations"
	"github.com/spf13/pflag"
)

var srcTmpl = `
package {{.Package}}

// This file has been generated - do not modify

import (
	"{{.API}}"
)

{{define "case"}}
	{{- $combo := .Combo}}
	{{- $prefix := .Prefix}}
	{{- $idx := 0}}

	{{- range $typ, $def := .Types}}
		{{- if gt $idx 0}} &&{{else}}{{$idx = 1}}{{end}} {{$def.VarName}} {{if contains $combo $typ}}!={{else}}=={{end}} nil
	{{- end}}:
		return &struct{
			{{.BaseType}}
{{- range $typ, $def := .Types}}
	{{- if contains $combo $typ}}
			{{$typ}}
	{{- end}}
{{- end}}
		}{
			{{.ShortBase}}: base,
{{- range $typ, $def := .Types}}
	{{- if contains $combo $typ}}
			{{$def.ShortType}}: &{{$prefix}}{{$def.ShortType}}Impl{
				{{$def.VarName}}: {{$def.VarName}},
			},
	{{- end}}
{{- end}}
		}
{{- end -}}

func {{.Function}}(base {{.BaseType}}{{range ordered}}, {{.VarName}} {{.Signature}}{{end}}) {{.BaseType}} {
{{- $basetype := .BaseType}}
{{- $shortbase := .ShortBase}}
{{- $prefix := .Function}}
{{- $types := .Types}}
{{- $idx := 0}}
	switch {
	case {{- range $typ, $def := .Types}}
		{{- if gt $idx 0}} &&{{else}}{{$idx = 1}}{{end}} {{$def.VarName}} == nil
	{{- end}}:
		return base
{{range $combo := .Combinations}}
	case {{- template "case" dict "BaseType" $basetype "Prefix" $prefix "ShortBase" $shortbase "Types" $types "Combo" $combo}}
{{end}}	}

	return nil
}

{{range .Types -}}
type {{$prefix}}{{.ShortType}}Impl struct {
	{{.VarName}} {{.Signature}}
}

func (impl *{{$prefix}}{{.ShortType}}Impl) {{.Function}}{{slice .Signature 4}} {
	return impl.{{.VarName}}()
}

{{end}}
`

type dynamicType struct {
	typ, function, signature string
}

type typeStruct struct {
	Type, ShortType, Signature, Function, VarName string
}

func generate(out io.Writer, packageName, functionName, baseType string, dynamicTypes ...dynamicType) {
	types := make(map[string]typeStruct, len(dynamicTypes))
	combos := make([]string, 0)

	tmpl, err := template.New("gen").Funcs(template.FuncMap{
		// dict combines key value pairs for passing structs into templates
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, errors.New("invalid dict call")
			}
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, errors.New("dict keys must be strings")
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},
		// contains checks if slice contains string
		"contains": func(combo []string, typ string) bool {
			for _, v := range combo {
				if v == typ {
					return true
				}
			}
			return false
		},
		// ordered checks if slice ordered string
		"ordered": func() []typeStruct {
			ordered := make([]typeStruct, 0)
			for _, k := range dynamicTypes {
				ordered = append(ordered, types[k.typ])
			}

			return ordered
		},
	}).Parse(srcTmpl)
	if err != nil {
		panic(err)
	}

	for _, dt := range dynamicTypes {
		parts := strings.SplitN(dt.typ, ".", 2)

		types[dt.typ] = typeStruct{
			Type:      dt.typ,
			ShortType: parts[1],
			VarName:   strings.ToLower(parts[1][:1]) + parts[1][1:],
			Signature: dt.signature,
			Function:  dt.function,
		}

		combos = append(combos, dt.typ)
	}

	baseTypeParts := strings.SplitN(baseType, ".", 2)
	vars := struct {
		API                 string
		Package, Function   string
		BaseType, ShortBase string
		Types               map[string]typeStruct
		Combinations        [][]string
	}{
		API:          "github.com/andig/evcc/api",
		Package:      packageName,
		Function:     functionName,
		BaseType:     baseType,
		ShortBase:    baseTypeParts[1],
		Types:        types,
		Combinations: combinations.All(combos),
	}

	if err := tmpl.Execute(out, vars); err != nil {
		println(err)
		os.Exit(2)
	}
}

var (
	target   = pflag.StringP("out", "o", "", "output file")
	pkg      = pflag.StringP("package", "p", "", "package name")
	function = pflag.StringP("function", "f", "decorate", "function name")
	base     = pflag.StringP("base", "b", "", "base type")
	types    = pflag.StringArrayP("type", "t", nil, "comma-separated list of type definitions")
)

// Usage prints flags usage
func Usage() {
	fmt.Fprintf(os.Stderr, "Usage of decorate:\n")
	fmt.Fprintf(os.Stderr, "\ndecorate [flags] -type interface,interface function,function signature\n")
	fmt.Fprintf(os.Stderr, "\nFlags:\n")
	pflag.PrintDefaults()
}

func main() {
	pflag.Usage = Usage
	pflag.Parse()

	if *base == "" || *pkg == "" || len(*types) == 0 {
		Usage()
		os.Exit(2)
	}

	var dynamicTypes []dynamicType
	for _, v := range *types {
		split := strings.SplitN(v, ",", 3)
		dt := dynamicType{split[0], split[1], split[2]}
		dynamicTypes = append(dynamicTypes, dt)
	}

	var buf bytes.Buffer
	generate(&buf, *pkg, *function, *base, dynamicTypes...)
	generated := strings.TrimSpace(buf.String()) + "\n"

	var out io.Writer = os.Stdout
	if *target != "" {
		name := *target
		if !strings.HasSuffix(name, ".go") {
			name += ".go"
		}

		dst, err := os.Create(name)
		if err != nil {
			println(err)
			os.Exit(2)
		}

		defer dst.Close()
		out = dst
	}

	if _, err := out.Write([]byte(generated)); err != nil {
		println(err)
		os.Exit(2)
	}
}
