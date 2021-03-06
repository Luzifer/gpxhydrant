package osm

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"time"
)

const (
	liveAPIBaseURL = "https://api.openstreetmap.org/api/0.6"
	devAPIBaseURL  = "https://api06.dev.openstreetmap.org/api/0.6"
)

// Client represents an OSM client which is capable of RW operations on the OpenStreetMap
type Client struct {
	username string
	password string

	APIBaseURL  string
	HTTPClient  *http.Client
	CurrentUser *User

	DebugHTTPRequests bool
}

// New instantiates a new client and retrieves information about the
// current user. Set useDevServer to true to change the API URL to the
// api06.dev.openstreetmap.org server.
func New(username, password string, useDevServer bool) (*Client, error) {
	if useDevServer {
		return NewWithAPIEndpoint(username, password, devAPIBaseURL)
	}
	return NewWithAPIEndpoint(username, password, liveAPIBaseURL)
}

// NewWithAPIEndpoint instantiates a new client and retrieves
// information about the current user. Set apiEndpoint to your desired API
// endpoint (e.g. https://api06.dev.openstreetmap.org/api/0.6)
func NewWithAPIEndpoint(username, password, apiEndpoint string) (*Client, error) {
	out := &Client{
		username: username,
		password: password,

		APIBaseURL: apiEndpoint,
		HTTPClient: http.DefaultClient,

		DebugHTTPRequests: false,
	}

	if apiEndpoint == "" {
		return nil, errors.New("No API endpoint given")
	}

	u := &Wrap{User: &User{}}
	if err := out.doParse("GET", "/user/details", nil, u); err != nil {
		return nil, err
	}
	out.CurrentUser = u.User

	return out, nil
}

func (c *Client) doPlain(method, path string, body io.Reader) (string, error) {
	responseBody, err := c.do(method, path, body)
	if err != nil {
		return "", err
	}
	defer responseBody.Close()

	data, err := ioutil.ReadAll(responseBody)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func (c *Client) do(method, path string, body io.Reader) (io.ReadCloser, error) {
	var reqBodyBuffer *bytes.Buffer
	if body != nil {
		reqBodyBuffer = new(bytes.Buffer)
		io.Copy(reqBodyBuffer, body)

		body = bytes.NewBuffer(reqBodyBuffer.Bytes())
	}

	req, _ := http.NewRequest(method, c.APIBaseURL+path, body)
	req.SetBasicAuth(c.username, c.password)

	if method != "GET" {
		req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	}

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	resBody := bytes.NewBufferString("")
	io.Copy(resBody, res.Body)

	if c.DebugHTTPRequests {
		buf := bytes.NewBufferString("")
		fmt.Fprintf(buf, "---------- REQUEST ----------\n")
		fmt.Fprintf(buf, "%s %s\n", method, req.URL.String())
		for k, v := range req.Header {
			fmt.Fprintf(buf, "%s: %s\n", k, v[0])
		}
		fmt.Fprintf(buf, "\n")
		if reqBodyBuffer != nil {
			trunc := int(math.Min(float64(reqBodyBuffer.Len()), 1024))
			fmt.Fprintf(buf, "%s\n\n", reqBodyBuffer.String()[0:trunc])
		}

		fmt.Fprintf(buf, "---------- RESPONSE ----------\n")
		for k, v := range res.Header {
			fmt.Fprintf(buf, "%s: %s\n", k, v[0])
		}
		fmt.Fprintf(buf, "\n")
		trunc := int(math.Min(float64(resBody.Len()), 1024))
		fmt.Fprintf(buf, "%s\n", resBody.String()[0:trunc])

		fmt.Println(buf.String())
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OSM API responded with status code %d", res.StatusCode)
	}

	return ioutil.NopCloser(resBody), nil
}

func (c *Client) doParse(method, path string, body io.Reader, output interface{}) error {
	responseBody, err := c.do(method, path, body)
	if err != nil {
		return err
	}
	defer responseBody.Close()

	if output != nil {
		return xml.NewDecoder(responseBody).Decode(output)
	}

	return nil
}

// Wrap is a mostly internal used struct which holds requests to / responses from the API.
// You will get a Wrap object when querying map objects from the API
type Wrap struct {
	XMLName    xml.Name     `xml:"osm"`
	User       *User        `xml:"user,omitempty"`
	Changesets []*Changeset `xml:"changeset,omitempty"`
	Nodes      []*Node      `xml:"node,omitempty"`
}

// Changeset contains information about a changeset in the API. You need to create a changeset before submitting any changes to the API.
type Changeset struct {
	XMLName      xml.Name  `xml:"changeset"`
	ID           int64     `xml:"id,attr,omitempty"`
	User         string    `xml:"user,attr,omitempty"`
	UID          int64     `xml:"uid,attr,omitempty"`
	CreatedAt    time.Time `xml:"created_at,attr,omitempty"`
	ClosedAt     time.Time `xml:"closed_at,attr,omitempty"`
	Open         bool      `xml:"open,attr,omitempty"`
	MinLat       float64   `xml:"min_lat,attr,omitempty"`
	MinLon       float64   `xml:"min_lon,attr,omitempty"`
	MaxLat       float64   `xml:"max_lat,attr,omitempty"`
	MaxLon       float64   `xml:"max_lon,attr,omitempty"`
	CommentCount int64     `xml:"comments_count,attr,omitempty"`

	Tags []Tag `xml:"tag"`
}

// GetMyChangesets retrieves a list of (open) changesets from the API
func (c *Client) GetMyChangesets(onlyOpen bool) ([]*Changeset, error) {
	urlPath := fmt.Sprintf("/changesets?user=%d&open=%s", c.CurrentUser.ID, strconv.FormatBool(onlyOpen))

	r := &Wrap{}
	return r.Changesets, c.doParse("GET", urlPath, nil, r)
}

// CreateChangeset creates a new changeset
func (c *Client) CreateChangeset() (*Changeset, error) {
	body := bytes.NewBufferString(xml.Header)

	enc := xml.NewEncoder(body)
	enc.Indent("", " ")

	if err := enc.Encode(Wrap{Changesets: []*Changeset{{}}}); err != nil {
		return nil, err
	}

	res, err := c.doPlain("PUT", "/changeset/create", body)
	if err != nil {
		return nil, err
	}

	cs := &Wrap{}
	if err := c.doParse("GET", fmt.Sprintf("/changeset/%s", res), nil, cs); err != nil {
		return nil, err
	}

	if len(cs.Changesets) != 1 {
		return nil, fmt.Errorf("Unable to retrieve new changeset #%s", res)
	}

	return cs.Changesets[0], nil
}

// SaveChangeset updates or creates a changeset
func (c *Client) SaveChangeset(cs *Changeset) error {
	urlPath := "/changeset/create"

	if cs.ID > 0 {
		urlPath = fmt.Sprintf("/changeset/%d", cs.ID)
	}

	data := Wrap{Changesets: []*Changeset{cs}}

	body := bytes.NewBufferString(xml.Header)

	enc := xml.NewEncoder(body)
	enc.Indent("", " ")

	if err := enc.Encode(data); err != nil {
		return err
	}

	_, err := c.doPlain("PUT", urlPath, body)
	return err
}

// RetrieveMapObjects queries all objects within the passed bounds. You need to ensure the min values are below the max values.
func (c *Client) RetrieveMapObjects(minLat, minLon, maxLat, maxLon float64) (*Wrap, error) {
	urlPath := fmt.Sprintf("/map?bbox=%.7f,%.7f,%.7f,%.7f", minLat, minLon, maxLat, maxLon)
	res := &Wrap{}
	return res, c.doParse("GET", urlPath, nil, res)
}

// User contains information about an User in the OpenStreetMap
type User struct {
	XMLName        xml.Name  `xml:"user"`
	ID             int64     `xml:"id,attr"`
	DisplayName    string    `xml:"display_name,attr"`
	AccountCreated time.Time `xml:"account_created,attr"`

	Description string `xml:"description"`
}

// Node represents one node in the OpenStreetMap
type Node struct {
	XMLName   xml.Name `xml:"node"`
	ID        int64    `xml:"id,attr,omitempty"`
	Version   int64    `xml:"version,attr,omitempty"`
	Changeset int64    `xml:"changeset,attr,omitempty"`
	User      string   `xml:"user,attr,omitempty"`
	UID       int64    `xml:"uid,attr,omitempty"`
	Latitude  float64  `xml:"lat,attr"`
	Longitude float64  `xml:"lon,attr"`

	Tags []Tag `xml:"tag"`
}

// SaveNode creates or updates a node with an association to the passed changeset which needs to be open and known to the API.
func (c *Client) SaveNode(n *Node, cs *Changeset) error {
	if n.ID > 0 && n.Version == 0 {
		return fmt.Errorf("When an ID is set the version must be present")
	}

	urlPath := "/node/create"

	if n.ID > 0 {
		urlPath = fmt.Sprintf("/node/%d", n.ID)
	}

	n.Changeset = cs.ID

	data := Wrap{Nodes: []*Node{n}}

	body := bytes.NewBufferString(xml.Header)

	enc := xml.NewEncoder(body)
	enc.Indent("", " ")

	if err := enc.Encode(data); err != nil {
		return err
	}

	_, err := c.doPlain("PUT", urlPath, body)
	return err
}

// Tag represents a key-value pair used in all objects inside OpenStreetMap
type Tag struct {
	XMLName xml.Name `xml:"tag"`
	Key     string   `xml:"k,attr"`
	Value   string   `xml:"v,attr"`
}
