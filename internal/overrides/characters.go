package overrides

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// StaffOverride is an authored staff file (config/overrides/staff/). Staff are
// global, so they live in their own directory rather than under a series.
type StaffOverride struct {
	// Path is the file's path relative to the overrides directory.
	Path string `yaml:"-"`

	Staff []model.Staff `yaml:"staff,omitempty"`
}

// IDs returns every staff id declared in the file.
func (o StaffOverride) IDs() []string {
	ids := make([]string, 0, len(o.Staff))
	for _, s := range o.Staff {
		ids = append(ids, s.ID)
	}
	return ids
}

// Validate checks every declared staff entry has an id.
func (o StaffOverride) Validate() error {
	if len(o.Staff) == 0 {
		return fmt.Errorf("override %q: declares no staff", o.Path)
	}
	for i := range o.Staff {
		if o.Staff[i].ID == "" {
			return fmt.Errorf("override %q: a staff entry has no id", o.Path)
		}
	}
	return nil
}

// ParseStaff decodes a staff override from raw YAML.
func ParseStaff(raw []byte, relPath string) (StaffOverride, error) {
	var o StaffOverride
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&o); err != nil {
		return StaffOverride{}, fmt.Errorf("parse staff override %q: %w", relPath, err)
	}
	o.Path = relPath
	if err := o.Validate(); err != nil {
		return StaffOverride{}, err
	}
	return o, nil
}
