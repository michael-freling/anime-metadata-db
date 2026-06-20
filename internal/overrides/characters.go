package overrides

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// CharactersOverride is an authored R2 file declaring characters and/or staff
// (the appearance graph, voice-actor links and ids the open source can't
// express). The builder fills names from Wikidata.
type CharactersOverride struct {
	// Path is the file's path relative to the overrides directory.
	Path string `yaml:"-"`

	Characters []model.Character `yaml:"characters,omitempty"`
	Staff      []model.Staff     `yaml:"staff,omitempty"`
}

// IDs returns every character and staff id declared in the file.
func (o CharactersOverride) IDs() []string {
	ids := make([]string, 0, len(o.Characters)+len(o.Staff))
	for _, c := range o.Characters {
		ids = append(ids, c.ID)
	}
	for _, s := range o.Staff {
		ids = append(ids, s.ID)
	}
	return ids
}

// Validate checks every declared character and staff entry has an id.
func (o CharactersOverride) Validate() error {
	if len(o.Characters) == 0 && len(o.Staff) == 0 {
		return fmt.Errorf("override %q: declares neither characters nor staff", o.Path)
	}
	for i := range o.Characters {
		if o.Characters[i].ID == "" {
			return fmt.Errorf("override %q: a character has no id", o.Path)
		}
	}
	for i := range o.Staff {
		if o.Staff[i].ID == "" {
			return fmt.Errorf("override %q: a staff entry has no id", o.Path)
		}
	}
	return nil
}

// ParseCharacters decodes an R2 override from raw YAML.
func ParseCharacters(raw []byte, relPath string) (CharactersOverride, error) {
	var o CharactersOverride
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&o); err != nil {
		return CharactersOverride{}, fmt.Errorf("parse characters override %q: %w", relPath, err)
	}
	o.Path = relPath
	if err := o.Validate(); err != nil {
		return CharactersOverride{}, err
	}
	return o, nil
}
