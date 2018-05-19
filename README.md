[![Go Report Card](https://goreportcard.com/badge/github.com/Luzifer/gpxhydrant)](https://goreportcard.com/report/github.com/Luzifer/gpxhydrant)
![](https://badges.fyi/github/license/Luzifer/gpxhydrant)
![](https://badges.fyi/github/downloads/Luzifer/gpxhydrant)
![](https://badges.fyi/github/latest-release/Luzifer/gpxhydrant)

# Luzifer / gpxhydrant

`gpxhydrant` is a small helper utility to map and update hydrants in [OpenStreetMap](https://www.openstreetmap.org/) for example used in the [OpenFireMap](http://openfiremap.org/) and [OSMHydrant](https://www.osmhydrant.org/) projects. It takes a single GPX file containing waypoints (in my case exported using Garmin Basecamp from my etrex Legend) with special comments in the waypoints.

Those special comments are used to set meta information about the hydrant. For example the comment `SU100` (seen below in the example GPX) would describe a hydrant placed in the `sidewalk`, beeing an `underground` hydrant with a pipe diameter of `100` milimeters.

If you use this tool please refer to the guidelines on the ["Contribute map data" wiki page](http://wiki.openstreetmap.org/wiki/Contribute_map_data) and ensure the data you've recorded is as accurate as possible.

## Possible characters in the comments

- For the position there are 4 letters: `S = sidewalk`, `P = parking_lot`, `L = lane` and `G = green`.
- For the type there are also 4 letters: `U = underground`, `O = pillar`, `W = wall` and `P = pond`
- The diameter can be `?` for unknown or consist of 2 to 3 numeric characters (`60`, `80`, `100`, ...)

## Execution

The most simple execution would be this one:

```bash
$ gpxhydrant -f myfile.gpx --osm-user="..." --osm-pass="..."
```

In that case all defaults are used and hydrants up to 5m distant to the location from your GPX file would match that one you're currently importing. In order to have those defaults make sense you need to ensure the recorded position of the hydrant is accurate with less than 5m derivation and you're standing exactly on the position of the hydrant.

If no hydrant is matched a new one will be created. You can test all the actions which would be taken by executing the command using the `-n` flag. In that case no data will be written to the OpenStreetMap API.

## Example GPX

```xml
<?xml version="1.0"?>
<gpx xmlns="http://www.topografix.com/GPX/1/1" [...]>

  <metadata>
    <link href="http://www.garmin.com">
      <text>Garmin International</text>
    </link>
    <time>2016-05-06T22:45:37Z</time>
    <bounds maxlat="53.589635314419866" maxlon="9.738048668950796" minlat="53.570238249376416" minlon="9.699116991832852"/>
  </metadata>

  [...]

  <wpt lat="53.584518497809768" lon="9.727988876402378">
    <ele>23.4376220703125</ele>
    <time>2016-05-06T22:45:29Z</time>
    <name>027</name>
    <cmt>05-MAI-16 13:35:35
SU100</cmt>
    <sym>Flag, Blue</sym>
    <type>user</type>
    [...]
  </wpt>

  [...]

</gpx>
```
