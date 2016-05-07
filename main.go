package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"

	"github.com/Luzifer/go_helpers/position"
	"github.com/Luzifer/gpxhydrant/gpx"
	"github.com/Luzifer/gpxhydrant/osm"
	"github.com/Luzifer/rconfig"
)

var (
	cfg = struct {
		GPXFile        string `flag:"gpx-file,f" description:"File containing GPX waypoints"`
		NoOp           bool   `flag:"noop,n" default:"true" description:"Fetch data from OSM but do not write"`
		VersionAndExit bool   `flag:"version" default:"false" description:"Print version and exit"`
		Pressure       int64  `flag:"pressure" default:"4" description:"Pressure of the water grid"`
		Debug          bool   `flag:"debug,d" default:"false" description:"Enable debug logging"`
		OSM            struct {
			Username string `flag:"osm-user" description:"Username to log into OSM"`
			Password string `flag:"osm-pass" description:"Password for osm-user"`
			UseDev   bool   `flag:"osm-dev" default:"false" description:"Switch to dev API"`
		}
		MachRange int64  `flag:"match-range" default:"20" description:"Range of meters to match GPX hydrants to OSM nodes"`
		Comment   string `flag:"comment,c" default:"Added hydrants from GPX file" description:"Comment for the changeset"`
	}{}
	version = "dev"

	wrongGPXComment = errors.New("GPX comment does not match expected format")
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
		return nil, wrongGPXComment
	}

	matches := infoRegex.FindStringSubmatch(in.Comment)
	out := &hydrant{
		Name:      in.Name,
		Latitude:  roundPrec(in.Latitude, 7),
		Longitude: roundPrec(in.Longitude, 7),
		Pressure:  cfg.Pressure,
	}

	switch matches[1] {
	case "S":
		out.Position = "sidewalk"
	case "P":
		out.Position = "parking_lot"
	case "L":
		out.Position = "lane"
	case "G":
		out.Position = "green"
	}

	switch matches[2] {
	case "U":
		out.Type = "underground"
	case "O":
		out.Type = "pillar"
	case "W":
		out.Type = "wall"
	case "P":
		out.Type = "pond"
	}

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
			if t.Value == "fire_hydrant" {
				validFireHydrant = true
			}
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
		return nil, fmt.Errorf("Did not find required 'emergency=fire_hydrant' tag.")
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

func init() {
	rconfig.Parse(&cfg)

	if cfg.VersionAndExit {
		fmt.Printf("gpxhydrant %s\n", version)
	}

	if cfg.GPXFile == "" {
		log.Fatalf("gpx-file is a required parameter")
	}
}

func main() {
	gpsFile, err := os.Open(cfg.GPXFile)
	if err != nil {
		log.Fatalf("Unable to open your GPX file: %s", err)
	}
	defer gpsFile.Close()

	gpxData, err := gpx.ParseGPXData(gpsFile)
	if err != nil {
		log.Fatalf("Unable to parse your GPX file: %s", err)
	}

	hydrants := []*hydrant{}
	var (
		minLat         = 9999.0
		minLon         = 9999.0
		maxLat, maxLon float64
	)
	for _, wp := range gpxData.Waypoints {
		h, e := parseWaypoint(wp)
		if e != nil {
			if cfg.Debug || e != wrongGPXComment {
				log.Printf("Found waypoint not suitable for converting: %s (Reason: %s)", wp.Name, e)
			}
			continue
		}
		if cfg.Debug {
			log.Printf("Found a hydrant from waypoint %s: %#v", wp.Name, h)
		}
		hydrants = append(hydrants, h)

		if minLat > h.Latitude {
			minLat = h.Latitude
		}
		if maxLat < h.Latitude {
			maxLat = h.Latitude
		}
		if minLon > h.Longitude {
			minLon = h.Longitude
		}
		if maxLon < h.Longitude {
			maxLon = h.Longitude
		}
	}

	osmClient, err := osm.New(cfg.OSM.Username, cfg.OSM.Password, cfg.OSM.UseDev)
	if err != nil {
		log.Fatalf("Unable to log into OSM: %s", err)
	}

	changeSets, err := osmClient.GetMyChangesets(true)
	if err != nil {
		log.Fatalf("Unable to get changesets: %s", err)
	}

	var cs *osm.Changeset
	if len(changeSets) > 0 {
		cs = changeSets[0]
	} else {
		cs, err = osmClient.CreateChangeset()
		if err != nil {
			log.Fatalf("Unable to create changeset: %s", err)
		}
	}

	if cfg.Debug {
		log.Printf("Working on Changeset %d", cs.ID)
	}

	cs.Tags = []osm.Tag{
		{Key: "comment", Value: cfg.Comment},
		{Key: "created_by", Value: fmt.Sprintf("gpxhydrant %s", version)},
	}

	if err := osmClient.SaveChangeset(cs); err != nil {
		log.Fatalf("Unable to save changeset: %s", err)
	}

	border := 0.0009 // Equals ~100m using haversine formula
	mapData, err := osmClient.RetrieveMapObjects(minLon-border, minLat-border, maxLon+border, maxLat+border)
	if err != nil {
		log.Fatalf("Unable to get map data: %s", err)
	}

	if cfg.Debug {
		log.Printf("Retrieved %d nodes from map", len(mapData.Nodes))
	}

	availableHydrants := []*hydrant{}
	for _, n := range mapData.Nodes {
		h, e := fromNode(n)
		if e != nil {
			continue
		}

		availableHydrants = append(availableHydrants, h)
	}

	for _, h := range hydrants {
		var found *hydrant
		for _, a := range availableHydrants {
			dist := position.Haversine(h.Longitude, h.Latitude, a.Longitude, a.Latitude)
			if dist <= float64(cfg.MachRange)/1000.0 {
				found = a
			}
		}

		if found == nil {
			// No matched hydrant: Lets create one
			if cfg.NoOp {
				log.Printf("[NOOP] Would send a create to OSM (Changeset %d): %#v", cs.ID, h.ToNode())
			} else {
				osmClient.SaveNode(h.ToNode(), cs)
				if cfg.Debug {
					log.Printf("Created a hydrant: %s", h.Name)
				}
			}
			continue
		}

		if h.Diameter == 0 && found.Diameter > 0 {
			h.Diameter = found.Diameter
		}

		if h.Diameter == found.Diameter && h.Position == found.Position && h.Pressure == found.Pressure && h.Type == found.Type {
			if cfg.Debug {
				log.Printf("Found a good looking hydrant which needs no update: %#v", h)
			}
			// Everything matches, we don't care
			continue
		}

		h.ID = found.ID
		h.Version = found.Version
		if cfg.NoOp {
			log.Printf("[NOOP] Would send a change to OSM (Changeset %d): To=%#v From=%#v", cs.ID, h.ToNode(), found.ToNode())
		} else {
			osmClient.SaveNode(h.ToNode(), cs)
			if cfg.Debug {
				log.Printf("Changed a hydrant: %s", h.Name)
			}
		}
	}
}

func roundPrec(in float64, nd int) float64 {
	// Quite ugly but working way to reduce number of digits after decimal point
	o, _ := strconv.ParseFloat(strconv.FormatFloat(in, 'f', nd, 64), 64)
	return o
}
