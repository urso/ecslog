package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"

	"github.com/alecthomas/template"
	wordwrap "github.com/mitchellh/go-wordwrap"
	yaml "gopkg.in/yaml.v2"
)

type schema struct {
	Version    string
	Top        map[string]*namespace // toplevel namespaces
	Namespaces map[string]*namespace // all namespaces with full name
	Values     map[string]*value     // all values with full name in schema
}

type namespace struct {
	Parent *namespace

	Name        string
	FullName    string
	Description string

	Children []*namespace
	Values   []*value
}

type value struct {
	Parent      *namespace
	Type        typeInfo
	Name        string
	FullName    string
	Description string
}

type typeInfo struct {
	Package     string
	Name        string
	Constructor string
}

// definition represent in yaml file field specifications.
type definition struct {
	Name        string
	Type        string
	Description string
	Fields      []definition
}

var (
	strType   = typeInfo{Name: "string", Constructor: "String"}
	intType   = typeInfo{Name: "int", Constructor: "Int"}
	longType  = typeInfo{Name: "int64", Constructor: "Int64"}
	floatType = typeInfo{Name: "float64", Constructor: "Float64"}
	dateType  = typeInfo{Package: "time", Name: "time.Time", Constructor: "Time"}
	durType   = typeInfo{Package: "time", Name: "time.Duration", Constructor: "Dur"}
	objType   = typeInfo{Name: "map[string]interface{}", Constructor: "Any"}
	ipType    = typeInfo{Name: "string", Constructor: "String"}
	geoType   = typeInfo{Name: "string", Constructor: "String"}
)

var codeTmpl = `
	package {{ .packageName }}
	
	import (
		{{ range $key := .packages }}
		  "{{ $key }}"
		{{ end }}

		"github.com/urso/ecslog/fld"
	)

	type (
	{{ range $ns := .schema.Namespaces }}
	  ns{{ $ns.FullName | goName }} struct {
		  {{ range $sub := $ns.Children }}
			{{ $sub.Description | goComment }}
			{{ $sub.Name | goName }} ns{{ $sub.FullName | goName }}
			{{ end }}
		}
	{{ end }}
	)

	var (
	{{ range $ns := .schema.Top }}
	  // {{ $ns.Name | goName }} provides fields in the ECS {{ $ns.FullName }} namespace.
		{{ $ns.Description | goComment }}
	  {{ $ns.Name | goName }} = ns{{ $ns.FullName | goName }}{}
	{{ end }}
	)

	const Version = "{{ .schema.Version }}"

  func ecsField(key string, val fld.Value) fld.Field {
		return fld.Field{Key: key, Value: val, Standardized: true}
	}
		  
  func ecsAny(key string, val interface{}) fld.Field   { return ecsField(key, fld.ValAny(val)) }
	func ecsTime(key string, val time.Time) fld.Field    { return ecsField(key, fld.ValTime(val)) }
	func ecsDur(key string, val time.Duration) fld.Field { return ecsField(key, fld.ValDuration(val)) }
  func ecsString(key, val string) fld.Field            { return ecsField(key, fld.ValString(val)) }
  func ecsInt(key string, val int) fld.Field           { return ecsField(key, fld.ValInt(val)) }
  func ecsInt64(key string, val int64) fld.Field       { return ecsField(key, fld.ValInt64(val)) }
  func ecsFloat64(key string, val float64) fld.Field       { return ecsField(key, fld.ValFloat(val)) }

	{{ range $ns := .schema.Namespaces }}
	// ## {{ $ns.FullName }} fields

    {{ range $value := $ns.Values }}
		// {{ $value.Name | goName }} create the ECS complain '{{ $value.FullName}}' field.
		{{ $value.Description | goComment }}
		func (ns{{ $ns.FullName | goName }}) {{ $value.Name | goName }}(value {{ $value.Type.Name }}) fld.Field {
			  return ecs{{ $value.Type.Constructor }}("{{ $value.FullName }}", value)
		}
		{{ end }}
	{{ end }}
`

func main() {
	var (
		schemaDir string
		pkgName   string
		outFile   string
		version   string
		fmtCode   bool
	)

	log.SetFlags(0)
	flag.StringVar(&schemaDir, "schema", "", "Schema directory containing .yml files (required)")
	flag.StringVar(&pkgName, "pkg", "ecs", "Target package name")
	flag.StringVar(&outFile, "out", "", "Output directory (required)")
	flag.StringVar(&version, "version", "", "ECS version (required)")
	flag.BoolVar(&fmtCode, "fmt", false, "Format output")
	flag.Parse()

	checkFlag("schema", schemaDir)
	checkFlag("version", version)

	schema, err := loadSchema(version, schemaDir)
	if err != nil {
		log.Fatalf("Error loading schema: %+v", err)
	}

	contents, err := execTemplate(codeTmpl, pkgName, schema)
	if err != nil {
		log.Fatalf("Error creating code: %+v", err)
	}

	if fmtCode {
		contents, err = format.Source(contents)
		if err != nil {
			log.Fatalf("failed to format code: %v", err)
		}
	}

	if outFile != "" {
		err := ioutil.WriteFile(outFile, contents, 0600)
		if err != nil {
			log.Fatalf("failed to write file '%v': %v", outFile, err)
		}
	} else {
		fmt.Printf("%s\n", contents)
	}
}

func execTemplate(tmpl, pkgName string, schema *schema) ([]byte, error) {
	funcs := template.FuncMap{
		"goName":    goTypeName,
		"goComment": goCommentify,
	}

	// collect packages to be imported
	packages := map[string]string{}
	for _, val := range schema.Values {
		pkg := val.Type.Package
		if pkg != "" {
			packages[pkg] = pkg
		}
	}

	var buf bytes.Buffer
	t := template.Must(template.New("").Funcs(funcs).Parse(tmpl))
	err := t.Execute(&buf, map[string]interface{}{
		"packageName": pkgName,
		"packages":    packages,
		"schema":      schema,
	})
	if err != nil {
		return nil, fmt.Errorf("executing code template failed: %+v", err)
	}

	return buf.Bytes(), nil
}

func loadSchema(version, root string) (*schema, error) {
	defs, err := loadDefs(root)
	if err != nil {
		return nil, err
	}

	schema := buildSchema(version, flattenDefs("", defs))
	copyDescriptions(schema, "", defs)
	return schema, nil
}

func loadDefs(root string) ([]definition, error) {
	files, err := filepath.Glob(filepath.Join(root, "*.yml"))
	if err != nil {
		return nil, fmt.Errorf("finding yml files in '%v' failed: %+v", root, err)
	}

	// load definitions
	var defs []definition
	for _, file := range files {
		contents, err := ioutil.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("error reading file %v: %+v", file, err)
		}

		var fileDefs []definition
		if err := yaml.Unmarshal(contents, &fileDefs); err != nil {
			return nil, fmt.Errorf("error parsing file %v: %+v", file, err)
		}

		defs = append(defs, fileDefs...)
	}

	return defs, nil
}

func flattenDefs(path string, in []definition) map[string]typeInfo {
	filtered := map[string]typeInfo{}
	for i := range in {
		fld := &in[i]
		fldPath := fld.Name
		if path != "" {
			fldPath = fmt.Sprintf("%v.%v", path, fldPath)
		}

		if fld.Type != "group" {
			filtered[fldPath] = getType(fld.Type, fldPath)
		}

		for k, v := range flattenDefs(fldPath, fld.Fields) {
			filtered[k] = v
		}
	}
	return filtered
}

func buildSchema(version string, defs map[string]typeInfo) *schema {
	s := &schema{
		Version:    version,
		Top:        map[string]*namespace{},
		Namespaces: map[string]*namespace{},
		Values:     map[string]*value{},
	}

	for fullName, ti := range defs {
		fullName = normalizePath(fullName)
		name, path := splitPath(fullName)

		var current *namespace
		val := &value{
			Type:     ti,
			Name:     name,
			FullName: fullName,
		}
		s.Values[fullName] = val

		// iterate backwards through fully qualified and build namespaces.
		// Namespaces and values get dynamically interlinked
		for path != "" {
			fullPath := path
			name, path = splitPath(path)

			ns := s.Namespaces[fullPath]
			newNS := ns == nil
			if newNS {
				ns = &namespace{
					Name:     name,
					FullName: fullPath,
				}
				s.Namespaces[fullPath] = ns
			}

			if val != nil {
				// first parent namespace. Let's add the value and reset, so it won't be added to another namespace
				val.Parent = ns
				ns.Values = append(ns.Values, val)
				val = nil
			}
			if current != nil && current.Parent == nil {
				// was new namespace, lets insert and link it
				current.Parent = ns
				ns.Children = append(ns.Children, current)
			}

			if !newNS { // we found a common ancestor in the tree, let's stop early
				current = nil
				break
			}

			current = ns // advance to parent namespace
		}

		if current != nil {
			// new top level namespace:
			s.Top[current.Name] = current
		}
	}

	return s
}

func copyDescriptions(schema *schema, root string, defs []definition) {
	for i := range defs {
		def := &defs[i]
		fqName := def.Name
		if root != "" {
			fqName = fmt.Sprintf("%v.%v", root, fqName)
		}

		path := normalizePath(fqName)
		if path != "" && def.Description != "" {
			if def.Type == "group" {
				ns := schema.Namespaces[path]
				if ns == nil {
					panic(fmt.Sprintf("no namespace for: %v", path))
				}

				ns.Description = def.Description
			} else {
				val := schema.Values[path]
				if val == nil {
					panic(fmt.Sprintf("no value for: %v", path))
				}

				val.Description = def.Description
			}
		}

		copyDescriptions(schema, fqName, def.Fields)
	}
}

func splitPath(in string) (name, parent string) {
	idx := strings.LastIndexByte(in, '.')
	if idx < 0 {
		return in, ""
	}

	return in[idx+1:], in[:idx]
}

func normalizePath(in string) string {
	var rootPaths = []string{"base", "ecs"}

	for _, path := range rootPaths {
		if in == path {
			return ""
		}
		if strings.HasPrefix(in, path) && len(in) > len(path) && in[len(path)] == '.' {
			return in[len(path)+1:]
		}
	}
	return in
}

func checkFlag(name, s string) {
	if s == "" {
		log.Fatalf("Error: -%v required", name)
	}
}

func getType(typ, name string) typeInfo {
	switch typ {
	case "keyword", "text":
		return strType
	case "integer":
		return intType
	case "long":
		return longType
	case "float":
		return floatType
	case "date":
		return dateType
	case "duration":
		return durType
	case "object":
		return objType
	case "ip":
		return ipType
	case "geo_point":
		return geoType
	default:
		panic(fmt.Sprintf("unknown type '%v' in field '%v'", typ, name))
	}
}

func goCommentify(s string) string {
	s = strings.Join(strings.Split(s, "\n"), " ")
	textLength := 75 - len(strings.Replace("", "\t", "    ", 4)+" // ")
	lines := strings.Split(wordwrap.WrapString(s, uint(textLength)), "\n")

	if len(lines) > 0 {
		// Remove empty first line.
		if strings.TrimSpace(lines[0]) == "" {
			lines = lines[1:]
		}
	}
	if len(lines) > 0 {
		// Remove empty last line.
		if strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
	}

	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}

	// remove empty lines
	for i := len(lines) - 1; i >= 0; i-- {
		if len(lines[i]) == 0 {
			lines = lines[:i]
		}
		break
	}

	for i := range lines {
		lines[i] = "// " + lines[i]
	}

	return strings.Join(lines, "\n")
}

func goTypeName(name string) string {
	var b strings.Builder
	for _, w := range strings.FieldsFunc(name, isSeparator) {
		b.WriteString(strings.Title(abbreviations(w)))
	}
	return b.String()
}

// abbreviations capitalizes common abbreviations.
func abbreviations(abv string) string {
	switch strings.ToLower(abv) {
	case "id", "ppid", "pid", "mac", "ip", "iana", "uid", "ecs", "url", "os", "http":
		return strings.ToUpper(abv)
	default:
		return abv
	}
}

// isSeparate returns true if the character is a field name separator. This is
// used to detect the separators in fields like ephemeral_id or instance.name.
func isSeparator(c rune) bool {
	switch c {
	case '.', '_':
		return true
	case '@':
		// This effectively filters @ from field names.
		return true
	default:
		return false
	}
}
