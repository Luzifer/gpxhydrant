package gpx

import (
	"encoding/xml"
	"io"
	"time"
)

// GPX represents the contents of an GPX file
type GPX struct {
	XMLName  xml.Name `xml:"gpx"`
	Metadata struct {
		Link struct {
			Href string `xml:"href,attr"`
			Text string `xml:"text"`
		} `xml:"link"`
		Time   time.Time `xml:"time"`
		Bounds struct {
			MaxLat float64 `xml:"maxlat,attr"`
			MaxLon float64 `xml:"maxlon,attr"`
			MinLat float64 `xml:"minlat,attr"`
			MinLon float64 `xml:"minlon,attr"`
		} `xml:"bounds"`
	} `xml:"metadata"`
	Waypoints []Waypoint `xml:"wpt"`
}

// Waypoint represents a single waypoint inside a GPX file
type Waypoint struct {
	XMLName     xml.Name  `xml:"wpt"`
	Latitude    float64   `xml:"lat,attr"`
	Longitude   float64   `xml:"lon,attr"`
	Elevation   float64   `xml:"ele"`
	Time        time.Time `xml:"time"`
	Name        string    `xml:"name"`
	Comment     string    `xml:"cmt"`
	Description string    `xml:"desc"`
	Symbol      string    `xml:"sym"`
	Type        string    `xml:"type"`
}

// ParseGPXData reads the contents of the GPX file and returns a parsed version
func ParseGPXData(in io.Reader) (*GPX, error) {
	out := &GPX{}
	return out, xml.NewDecoder(in).Decode(out)
}
