package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OpenStreetMap vector data from the Overpass API, cached on disk. Each fetch
// asks for the ways (streets, buildings, water, parks, rail) within a radius
// of a point, which sends that approximate position to overpass-api.de.

const overpassURL = "https://overpass-api.de/api/interpreter"

type wayKind int

const (
	kindPark wayKind = iota
	kindWater
	kindBuilding
	kindRail
	kindFoot
	kindMinor
	kindMajor
)

type latLon struct{ Lat, Lon float64 }

type way struct {
	kind   wayKind
	closed bool
	name   string
	pts    []latLon
}

type osmResponse struct {
	Elements []struct {
		Tags     map[string]string `json:"tags"`
		Geometry []latLon          `json:"geometry"`
	} `json:"elements"`
}

func (l *latLon) UnmarshalJSON(b []byte) error {
	var v struct{ Lat, Lon float64 }
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	l.Lat, l.Lon = v.Lat, v.Lon
	return nil
}

func classify(t map[string]string) (wayKind, bool) {
	switch hw := t["highway"]; hw {
	case "":
	case "motorway", "trunk", "primary", "motorway_link", "trunk_link", "primary_link":
		return kindMajor, true
	case "footway", "path", "pedestrian", "cycleway", "steps", "track", "bridleway":
		return kindFoot, true
	default:
		return kindMinor, true
	}
	switch {
	case t["building"] != "":
		return kindBuilding, true
	case t["natural"] == "water" || t["waterway"] != "":
		return kindWater, true
	case t["leisure"] == "park" || t["landuse"] == "grass" || t["landuse"] == "forest" || t["landuse"] == "meadow":
		return kindPark, true
	case t["railway"] == "rail" || t["railway"] == "tram":
		return kindRail, true
	}
	return 0, false
}

// asciiFold turns a name into plain ASCII for the terminal: accents are
// dropped (á -> a, ñ -> n, ü -> u, ß -> ss), anything else becomes '?'.
func asciiFold(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 128:
			b.WriteRune(r)
		case strings.ContainsRune("ÀÁÂÃÄÅàáâãäå", r):
			b.WriteByte("Aa"[boolIdx(r >= 'à')])
		case strings.ContainsRune("ÈÉÊËèéêë", r):
			b.WriteByte("Ee"[boolIdx(r >= 'è')])
		case strings.ContainsRune("ÌÍÎÏìíîï", r):
			b.WriteByte("Ii"[boolIdx(r >= 'ì')])
		case strings.ContainsRune("ÒÓÔÕÖØòóôõöø", r):
			b.WriteByte("Oo"[boolIdx(r >= 'ò')])
		case strings.ContainsRune("ÙÚÛÜùúûü", r):
			b.WriteByte("Uu"[boolIdx(r >= 'ù')])
		case r == 'Ñ':
			b.WriteByte('N')
		case r == 'ñ':
			b.WriteByte('n')
		case r == 'Ç':
			b.WriteByte('C')
		case r == 'ç':
			b.WriteByte('c')
		case r == 'ß':
			b.WriteString("ss")
		case r == '\u2019' || r == '\u2018':
			b.WriteByte('\'')
		case r == '\u2013' || r == '\u2014':
			b.WriteByte('-')
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}

func boolIdx(b bool) int {
	if b {
		return 1
	}
	return 0
}

func cacheDir() string {
	d, err := os.UserCacheDir()
	if err != nil {
		d = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	return filepath.Join(d, "sensordemo", "osm")
}

// fetchWays returns the ways within radius meters of center, from the disk
// cache when this exact area was fetched before.
func fetchWays(center latLon, radius int) ([]way, error) {
	name := fmt.Sprintf("%.4f_%.4f_%d.json", center.Lat, center.Lon, radius)
	path := filepath.Join(cacheDir(), name)
	data, err := os.ReadFile(path)
	if err != nil {
		around := fmt.Sprintf("(around:%d,%.6f,%.6f)", radius, center.Lat, center.Lon)
		q := "[out:json][timeout:25];("
		for _, f := range []string{`["highway"]`, `["building"]`, `["natural"="water"]`, `["waterway"]`,
			`["leisure"="park"]`, `["landuse"~"^(grass|forest|meadow)$"]`, `["railway"~"^(rail|tram)$"]`} {
			q += "way" + f + around + ";"
		}
		q += ");out geom qt;"
		req, _ := http.NewRequest("POST", overpassURL, strings.NewReader("data="+url.QueryEscape(q)))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", "sensordemo/0.1 (personal terminal map demo)")
		client := &http.Client{Timeout: 45 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("overpass: %s", resp.Status)
		}
		if data, err = io.ReadAll(resp.Body); err != nil {
			return nil, err
		}
		os.MkdirAll(cacheDir(), 0o700)
		os.WriteFile(path, data, 0o600)
	}
	var r osmResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	var ways []way
	for _, e := range r.Elements {
		k, ok := classify(e.Tags)
		if !ok || len(e.Geometry) < 2 {
			continue
		}
		g := e.Geometry
		closed := len(g) > 3 && g[0] == g[len(g)-1] && k <= kindBuilding
		ways = append(ways, way{kind: k, closed: closed, name: asciiFold(e.Tags["name"]), pts: g})
	}
	return ways, nil
}
