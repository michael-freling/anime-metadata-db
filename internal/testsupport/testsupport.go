// Package testsupport provides shared fixtures and a fake fetcher for tests of
// the builder's higher-level packages (app, cmd). It is test-only but lives in
// a normal package so multiple test packages can import it.
package testsupport

import (
	"context"
	"fmt"
	"strings"
)

// OfflineDBJSON is a minimal anime-offline-database covering the Demon Slayer
// nodes referenced by DemonSlayerOverride, plus one AniList-less entry that must
// be skipped on load.
const OfflineDBJSON = `{
  "data": [
    {
      "sources": ["https://anilist.co/anime/101922", "https://anidb.net/anime/14353"],
      "title": "Kimetsu no Yaiba",
      "type": "TV",
      "episodes": 26,
      "animeSeason": { "season": "SPRING", "year": 2019 },
      "synonyms": ["鬼滅の刃", "Demon Slayer: Kimetsu no Yaiba"]
    },
    {
      "sources": ["https://anilist.co/anime/142984", "https://anidb.net/anime/16182"],
      "title": "Kimetsu no Yaiba: Mugen Ressha-hen (TV)",
      "type": "TV",
      "episodes": 7,
      "animeSeason": { "season": "FALL", "year": 2021 },
      "synonyms": ["鬼滅の刃 無限列車編"]
    },
    {
      "sources": ["https://anilist.co/anime/112151", "https://anidb.net/anime/15183"],
      "title": "Kimetsu no Yaiba: Mugen Ressha-hen",
      "type": "MOVIE",
      "episodes": 1,
      "animeSeason": { "season": "FALL", "year": 2020 },
      "synonyms": ["劇場版 鬼滅の刃 無限列車編"]
    },
    {
      "sources": ["https://anilist.co/anime/178680", "https://anidb.net/anime/18000"],
      "title": "Kimetsu no Yaiba: Mugen Jou-hen",
      "type": "MOVIE",
      "episodes": 1,
      "animeSeason": { "season": "SUMMER", "year": 2025 },
      "synonyms": ["無限城編"]
    },
    {
      "sources": ["https://myanimelist.net/anime/99999"],
      "title": "No AniList Entry",
      "type": "TV",
      "episodes": 1
    }
  ]
}`

// AnimeListXML maps a couple of the AniDB ids onto TVDB entries so the build can
// cross-fill tvdbId.
const AnimeListXML = `<?xml version="1.0" encoding="UTF-8"?>
<anime-list>
  <anime anidbid="14353" tvdbid="361069" defaulttvdbseason="1" episodeoffset="0">
    <name>Kimetsu no Yaiba</name>
  </anime>
  <anime anidbid="16182" tvdbid="361069" defaulttvdbseason="2" episodeoffset="26">
    <name>Mugen Train Arc</name>
  </anime>
  <anime anidbid="0" tvdbid="unknown">
    <name>Skipped</name>
  </anime>
</anime-list>`

// MovieSetXML groups the two Demon Slayer films into one movie set.
const MovieSetXML = `<?xml version="1.0" encoding="UTF-8"?>
<anime-movieset-list>
  <set name="Demon Slayer Movies">
    <anime anidbid="15183"/>
    <anime anidbid="18000"/>
  </set>
  <set name="Empty Set"/>
</anime-movieset-list>`

// DemonSlayerOverride is a numbered standalone series exercising seasons, an
// alternate-cut film (no number) and an original film (numbered, in a set).
const DemonSlayerOverride = `series:
  id: demon-slayer
  seasons:
    - id: ds-s1
      number: 1
      externalIds: { anilistId: 101922 }
    - id: ds-s2p1
      number: 2
      part: 1
      externalIds: { anilistId: 142984 }
  movies:
    - id: ds-mugen-film
      externalIds: { anilistId: 112151 }
      alternateCutOf: { seasonId: ds-s2p1, episodes: "1-7" }
    - id: ds-infinity
      externalIds: { anilistId: 178680 }
numbered: [demon-slayer]
`

// WikidataJSON is a wbgetentities-shaped fixture covering the QIDs referenced
// by CharactersOverride.
const WikidataJSON = `{
  "entities": {
    "Q2596113": {"id":"Q2596113","labels":{"en":{"language":"en","value":"Natsuki Hanae"},"ja":{"language":"ja","value":"花江夏樹"}}},
    "Q85805158": {"id":"Q85805158","labels":{"en":{"language":"en","value":"Tanjirō Kamado"},"ja":{"language":"ja","value":"竈門炭治郎"}}}
  },
  "success": 1
}`

// StaffOverride is a global staff file (separate from the series files).
const StaffOverride = `staff:
  - id: natsuki-hanae
    externalIds: { wikidataId: Q2596113 }
`

// DemonSlayerMerged is the Demon Slayer series file with its cast co-located:
// DemonSlayerOverride plus a character whose VA links to the staff above and the
// QIDs in WikidataJSON.
const DemonSlayerMerged = DemonSlayerOverride + `characters:
  - id: tanjiro-kamado
    externalIds: { wikidataId: Q85805158 }
    voiceActors:
      - { staffId: natsuki-hanae, language: ja }
    appearances:
      - seriesId: demon-slayer
        scope:
          - { seasonId: ds-s1 }
`

// FakeFetcher serves the fixtures above by matching the request URL, with hooks
// to simulate failures.
type FakeFetcher struct {
	// Err, when set, makes every Get fail with it.
	Err error
	// FailURL, when non-empty, makes Get fail for any URL containing it.
	FailURL string
}

// Get returns the fixture matching url, or an error per the configured hooks.
func (f FakeFetcher) Get(_ context.Context, url string) ([]byte, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	if f.FailURL != "" && strings.Contains(url, f.FailURL) {
		return nil, fmt.Errorf("fake fetch failed for %s", url)
	}
	switch {
	case strings.Contains(url, "wikidata"):
		return []byte(WikidataJSON), nil
	case strings.Contains(url, "offline"):
		return []byte(OfflineDBJSON), nil
	case strings.Contains(url, "movieset"):
		return []byte(MovieSetXML), nil
	case strings.Contains(url, "anime-list"):
		return []byte(AnimeListXML), nil
	default:
		return nil, fmt.Errorf("fake fetcher: unknown url %s", url)
	}
}
