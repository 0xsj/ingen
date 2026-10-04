module ingen

go 1.27.1

require (
	github.com/0xsj/ingen/amber v0.0.0-00010101000000-000000000000
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/0xsj/ingen/amber => ./amber/go
