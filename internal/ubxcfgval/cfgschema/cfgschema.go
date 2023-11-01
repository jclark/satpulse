package cfgschema

import (
	"gopkg.in/yaml.v3"
)

type Item struct {
	Key         uint32
	Type        string
	Description string
	Unit        string
	Scale       float64
	Note        string
	Min         float64
	Max         float64
	Condition   string
	Constants   map[string]Constant
}

type Constant struct {
	Value       uint64
	Description string
}

func UnmarshalYAML(bytes []byte) (map[string]map[string]Item, error) {
	var schema map[string]map[string]Item

	err := yaml.Unmarshal(bytes, &schema)
	if err != nil {
		return nil, err
	}
	return schema, nil
}
