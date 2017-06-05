package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/Luzifer/go_helpers/position"
	"github.com/Luzifer/gpxhydrant/gpx"
	"github.com/Luzifer/gpxhydrant/osm"
	"github.com/Luzifer/rconfig"
	log "github.com/Sirupsen/logrus"
)

var (
	cfg = struct {
		Comment   string `flag:"comment,c" default:"Added hydrants from GPX file" description:"Comment for the changeset"`
		Debug     bool   `flag:"debug,d" default:"false" description:"Enable debug logging (Deprecated: Use --log-level=debug)"`
		GPXFile   string `flag:"gpx-file,f" description:"File containing GPX waypoints"`
		LogLevel  string `flag:"log-level" default:"info" description:"Log level (debug, info, warn, error)"`
		MachRange int64  `flag:"match-range" default:"5" description:"Range of meters to match GPX hydrants to OSM nodes"`
		NoOp      bool   `flag:"noop,n" default:"false" description:"Fetch data from OSM but do not write"`
		OSM       struct {
			Username string `flag:"osm-user" description:"Username to log into OSM"`
			Password string `flag:"osm-pass" description:"Password for osm-user"`
			UseDev   bool   `flag:"osm-dev" default:"false" description:"Switch to dev API"`
		}
		Pressure       int64 `flag:"pressure" default:"4" description:"Pressure of the water grid"`
		VersionAndExit bool  `flag:"version" default:"false" description:"Print version and exit"`
	}{}
	version = "dev"

	changeset *osm.Changeset

	errWrongGPXComment = errors.New("GPX comment does not match expected format")
)

type bounds struct{ MinLat, MinLon, MaxLat, MaxLon float64 }

func (b *bounds) Update(lat, lon float64) {
	if b.MinLat > lat {
		b.MinLat = lat
	}
	if b.MaxLat < lat {
		b.MaxLat = lat
	}
	if b.MinLon > lon {
		b.MinLon = lon
	}
	if b.MaxLon < lon {
		b.MaxLon = lon
	}
}

func init() {
	rconfig.Parse(&cfg)

	if cfg.VersionAndExit {
		fmt.Printf("gpxhydrant %s\n", version)
		os.Exit(0)
	}

	if l, err := log.ParseLevel(cfg.LogLevel); err == nil {
		log.SetLevel(l)
	} else {
		log.Fatalf("Unable to parse log level: %s", err)
	}

	// Support deprecated parameter to overwrite log level
	if cfg.Debug {
		log.SetLevel(log.DebugLevel)
	}

	if cfg.GPXFile == "" {
		log.Fatalf("gpx-file is a required parameter")
	}

	if cfg.OSM.Password == "" || cfg.OSM.Username == "" {
		log.Fatalf("osm-pass / osm-user are required parameters")
	}
}

func hydrantsFromGPXFile() ([]*hydrant, bounds) {
	// Read and parse GPX file
	gpsFile, err := os.Open(cfg.GPXFile)
	if err != nil {
		log.Fatalf("Unable to open your GPX file: %s", err)
	}
	defer gpsFile.Close()

	gpxData, err := gpx.ParseGPXData(gpsFile)
	if err != nil {
		log.Fatalf("Unable to parse your GPX file: %s", err)
	}

	bds := bounds{MinLat: 9999, MinLon: 9999}
	hydrants := []*hydrant{}

	for _, wp := range gpxData.Waypoints {
		h, e := parseWaypoint(wp)
		if e != nil {
			if e != errWrongGPXComment {
				log.Debugf("Found waypoint not suitable for converting: %s (Reason: %s)", wp.Name, e)
			}
			continue
		}
		log.Debugf("Found a hydrant from waypoint %s: %#v", wp.Name, h)
		hydrants = append(hydrants, h)

		bds.Update(h.Latitude, h.Longitude)
	}

	return hydrants, bds
}

func createChangeset(osmClient *osm.Client) *osm.Changeset {
	if changeset != nil {
		return changeset
	}

	cs, err := osmClient.CreateChangeset()
	if err != nil {
		log.Fatalf("Unable to create changeset: %s", err)
	}

	log.Debugf("Working on Changeset %d", cs.ID)

	cs.Tags = []osm.Tag{
		//{Key: "comment", Value: cfg.Comment},
		{Key: "created_by", Value: fmt.Sprintf("gpxhydrant %s", version)},
	}

	if err := osmClient.SaveChangeset(cs); err != nil {
		log.Fatalf("Unable to save changeset: %s", err)
	}

	changeset = cs

	return cs
}

func getHydrantsFromOSM(osmClient *osm.Client, bds bounds) []*hydrant {
	border := 0.0009 // Equals ~100m using haversine formula
	mapData, err := osmClient.RetrieveMapObjects(bds.MinLon-border, bds.MinLat-border, bds.MaxLon+border, bds.MaxLat+border)
	if err != nil {
		log.Fatalf("Unable to get map data: %s", err)
	}

	log.Debugf("Retrieved %d nodes from map", len(mapData.Nodes))

	availableHydrants := []*hydrant{}
	for _, n := range mapData.Nodes {
		h, e := fromNode(n)
		if e != nil {
			continue // Not a hydrant, ignore that node
		}

		availableHydrants = append(availableHydrants, h)
	}

	return availableHydrants
}

func main() {
	// Convert waypoints from GPX file to hydrants
	hydrants, bds := hydrantsFromGPXFile()

	osmClient, err := osm.New(cfg.OSM.Username, cfg.OSM.Password, cfg.OSM.UseDev)
	if err != nil {
		log.Fatalf("Unable to log into OSM: %s", err)
	}

	osmClient.DebugHTTPRequests = log.GetLevel() == log.DebugLevel

	// Retrieve currently available information from OSM
	availableHydrants := getHydrantsFromOSM(osmClient, bds)

	updateOrCreateHydrants(hydrants, availableHydrants, osmClient)
}

func updateOrCreateHydrants(hydrants, availableHydrants []*hydrant, osmClient *osm.Client) {
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
			doNoOp(
				fmt.Sprintf("[NOOP] Would send a create to OSM (Changeset %d): %#v", createChangeset(osmClient).ID, h.ToNode()),
				func() {
					if err := osmClient.SaveNode(h.ToNode(), createChangeset(osmClient)); err != nil {
						log.Fatalf("Unable to create node using the OSM API: %s", err)
					}
					log.Debugf("Created a hydrant: %s", h.Name)
				},
			)
			continue
		}

		// Special case: If the diameter of the recorded hydrant is unknown but previously known keep the previous version
		if h.Diameter == 0 && found.Diameter > 0 {
			h.Diameter = found.Diameter
		}

		if !found.NeedsUpdate(h) {
			log.Debugf("Found a good looking hydrant which needs no update: %#v", h)
			// Everything matches, we don't care
			continue
		}

		h.ID = found.ID
		h.Version = found.Version
		doNoOp(
			fmt.Sprintf("[NOOP] Would send a change to OSM (Changeset %d): To=%#v From=%#v", createChangeset(osmClient).ID, h.ToNode(), found.ToNode()),
			func() {
				if err := osmClient.SaveNode(h.ToNode(), createChangeset(osmClient)); err != nil {
					log.Fatalf("Unable to create node using the OSM API: %s", err)
				}
				log.Debugf("Changed a hydrant: %s", h.Name)
			},
		)
	}
}

func doNoOp(message string, execution func()) {
	if cfg.NoOp {
		log.Println(message)
		return
	}

	execution()
}

func roundPrec(in float64, nd int) float64 {
	// Quite ugly but working way to reduce number of digits after decimal point
	o, _ := strconv.ParseFloat(strconv.FormatFloat(in, 'f', nd, 64), 64)
	return o
}
