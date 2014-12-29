go-typeswitch-gen
=================

## INSTALLATION

    go get github.com/motemen/go-typeswitch-gen/cmd/tsgen

## USAGE

    tsgen FILES...

    tsgen -type T=int,*foo.Bar,map[string]bool FILES...

## SOURCE CODE TEMPLATES

    //go:generate tsgen example.go

    type T interface{}

    func mapKeys(m interface{}) []string {
            // +tsgen T:"int,bool,[]int"
            switch m := m.(type) {
            default:
                    panic(fmt.Sprintf("unexpected value of type %T", m))

            case map[string]T:
                    keys := make([]string, 0, len(m))
                    for key := range m {
                            keys = append(keys, key)
                    }
                    return keys
            }
    }


## DESCRIPTION

`tsgen` rewrites Go source files in which the type switch statements with case clauses including type variables (e.g. `map[string]T` or `chan S1`)
are instantiated with concrete types (e.g. `map[string]io.Reader` or `chan []byte`)

Types with names of uppercase letters and numbers are concidered as type variables.
