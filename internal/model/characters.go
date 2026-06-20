package model

// The R2 model: Characters and Staff (voice actors). Both are GLOBAL,
// many-to-many nodes that attach onto the R1 series spine through a Character's
// appearances. We store facts (ids, names, the appearance graph, voice-actor
// links); expression (roles, bios, images) is fetched live by the consumer.

// VoiceActor links a character to the Staff who voices it in a language.
type VoiceActor struct {
	StaffID  string `yaml:"staffId"`
	Language string `yaml:"language"`
}

// ScopeRef narrows a CharacterAppearance to a specific node of a Series.
// Exactly one of the fields is set.
type ScopeRef struct {
	SeasonID  string `yaml:"seasonId,omitempty"`
	MovieID   string `yaml:"movieId,omitempty"`
	SpecialID string `yaml:"specialId,omitempty"`
}

// CharacterAppearance is a Character ↔ Series edge. SeriesID is the rollup
// association; Scope optionally narrows it to specific nodes; VoiceActors
// optionally overrides the character's default cast for this appearance.
type CharacterAppearance struct {
	SeriesID    string       `yaml:"seriesId"`
	Scope       []ScopeRef   `yaml:"scope,omitempty"`
	VoiceActors []VoiceActor `yaml:"voiceActors,omitempty"`
	ExternalIDs ExternalIDs  `yaml:"externalIds,omitempty"`
}

// Character is a global fictional entity, owned by no Franchise or Series.
type Character struct {
	ID          string                `yaml:"id"`
	Names       Title                 `yaml:"names,omitempty"`
	ExternalIDs ExternalIDs           `yaml:"externalIds,omitempty"`
	VoiceActors []VoiceActor          `yaml:"voiceActors,omitempty"`
	Appearances []CharacterAppearance `yaml:"appearances,omitempty"`
}

// Staff is a global real person — currently only voice actors.
type Staff struct {
	ID          string      `yaml:"id"`
	Names       Title       `yaml:"names,omitempty"`
	ExternalIDs ExternalIDs `yaml:"externalIds,omitempty"`
}

// StaffRecord is one generated staff dataset file (data/staff/). Staff are
// global, so they live apart from the series records.
type StaffRecord struct {
	Staff []Staff `yaml:"staff,omitempty"`
}
