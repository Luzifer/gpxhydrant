package main

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/Luzifer/gpxhydrant/gpx"
	"github.com/Luzifer/gpxhydrant/osm"
)

var (
	hydrantPositions = map[string]string{
		"S": "sidewalk",
		"P": "parking_lot",
		"L": "lane",
		"G": "green",
	}
	hydrantTypes = map[string]string{
		"U": "underground",
		"O": "pillar",
		"W": "wall",
		"P": "pond",
	}
)

type hydrant struct {
	/*
	   <node lat="53.58963" lon="9.70838">
	     <tag k="emergency" v="fire_hydrant" />
	     <tag k="fire_hydrant:diameter" v="100" />
	     <tag k="fire_hydrant:position" v="sidewalk" />
	     <tag k="fire_hydrant:pressure" v="4" />
	     <tag k="fire_hydrant:type" v="underground" />
	     <tag k="operator" v="Stadtwerke Wedel" />
	   </node>
	*/
	ID        int64
	Name      string
	Latitude  float64
	Longitude float64
	Diameter  int64
	Position  string
	Pressure  int64
	Type      string
	Version   int64
}

func parseWaypoint(in gpx.Waypoint) (*hydrant, error) {
	infoRegex := regexp.MustCompile(`([SPLG])([UOWP])(\?|[0-9]{2,3})`)
	if !infoRegex.MatchString(in.Comment) {
		return nil, errWrongGPXComment
	}

	matches := infoRegex.FindStringSubmatch(in.Comment)
	out := &hydrant{
		Name:      in.Name,
		Latitude:  roundPrec(in.Latitude, 7),
		Longitude: roundPrec(in.Longitude, 7),
		Pressure:  cfg.Pressure,
	}

	out.Position = hydrantPositions[matches[1]]
	out.Type = hydrantTypes[matches[2]]

	if matches[3] != "?" {
		diameter, err := strconv.ParseInt(matches[3], 10, 64)
		if err != nil {
			return nil, err
		}
		out.Diameter = diameter
	}

	return out, nil
}

func fromNode(in *osm.Node) (*hydrant, error) {
	var e error

	out := &hydrant{
		ID:        in.ID,
		Version:   in.Version,
		Latitude:  in.Latitude,
		Longitude: in.Longitude,
	}

	validFireHydrant := false

	for _, t := range in.Tags {
		switch t.Key {
		case "emergency":
			validFireHydrant = t.Value == "fire_hydrant"
		case "fire_hydrant:diameter":
			if out.Diameter, e = strconv.ParseInt(t.Value, 10, 64); e != nil {
				return nil, e
			}
		case "fire_hydrant:position":
			out.Position = t.Value
		case "fire_hydrant:pressure":
			if out.Pressure, e = strconv.ParseInt(t.Value, 10, 64); e != nil {
				return nil, e
			}
		case "fire_hydrant:type":
			out.Type = t.Value
		}
	}

	if !validFireHydrant {
		return nil, fmt.Errorf("did not find required 'emergency=fire_hydrant' tag")
	}

	return out, nil
}

func (h hydrant) ToNode() *osm.Node {
	out := &osm.Node{
		ID:        h.ID,
		Version:   h.Version,
		Latitude:  h.Latitude,
		Longitude: h.Longitude,
	}

	out.Tags = append(out.Tags, osm.Tag{Key: "emergency", Value: "fire_hydrant"})
	if h.Diameter > 0 {
		out.Tags = append(out.Tags, osm.Tag{Key: "fire_hydrant:diameter", Value: strconv.FormatInt(h.Diameter, 10)})
	}
	out.Tags = append(out.Tags, osm.Tag{Key: "fire_hydrant:position", Value: h.Position})
	out.Tags = append(out.Tags, osm.Tag{Key: "fire_hydrant:pressure", Value: strconv.FormatInt(h.Pressure, 10)})
	out.Tags = append(out.Tags, osm.Tag{Key: "fire_hydrant:type", Value: h.Type})

	return out
}

func (h hydrant) NeedsUpdate(in *hydrant) bool {
	return h.Diameter != in.Diameter || h.Position != in.Position || h.Pressure != in.Pressure || h.Type != in.Type
}
