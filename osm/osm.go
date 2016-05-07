package osm

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"
)

const (
	liveAPIBaseURL = "http://api.openstreetmap.org/api/0.6"
	devAPIBaseURL  = "http://api06.dev.openstreetmap.org/api/0.6"
)

type Client struct {
	username string
	password string

	APIBaseURL  string
	HTTPClient  *http.Client
	CurrentUser *User
}

func New(username, password string, useDevServer bool) (*Client, error) {
	out := &Client{
		username: username,
		password: password,

		APIBaseURL: liveAPIBaseURL,
		HTTPClient: http.DefaultClient,
	}

	if useDevServer {
		out.APIBaseURL = devAPIBaseURL
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
	req, _ := http.NewRequest(method, c.APIBaseURL+path, body)
	req.SetBasicAuth(c.username, c.password)

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		d, e := ioutil.ReadAll(res.Body)
		if e != nil {
			return nil, fmt.Errorf("OSM API responded with status code %d and reading response failed.", res.StatusCode)
		}

		res.Body.Close()
		return nil, fmt.Errorf("OSM API responded with status code %d (%s)", res.StatusCode, d)
	}

	return res.Body, nil
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

type Wrap struct {
	XMLName    xml.Name     `xml:"osm"`
	User       *User        `xml:"user,omitempty"`
	Changesets []*Changeset `xml:"changeset,omitempty"`
	Nodes      []*Node      `xml:"node,omitempty"`
}

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

func (c *Client) GetMyChangesets(onlyOpen bool) ([]*Changeset, error) {
	urlPath := fmt.Sprintf("/changesets?user=%d&open=%s", c.CurrentUser.ID, strconv.FormatBool(onlyOpen))

	r := &Wrap{}
	return r.Changesets, c.doParse("GET", urlPath, nil, r)
}

func (c *Client) CreateChangeset(comment string) (*Changeset, error) {
	body := bytes.NewBuffer([]byte{})
	if err := xml.NewEncoder(body).Encode(Wrap{Changesets: []*Changeset{{}}}); err != nil {
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

func (c *Client) RetrieveMapObjects(minLat, minLon, maxLat, maxLon float64) (*Wrap, error) {
	urlPath := fmt.Sprintf("/map?bbox=%.7f,%.7f,%.7f,%.7f", minLat, minLon, maxLat, maxLon)
	res := &Wrap{}
	return res, c.doParse("GET", urlPath, nil, res)
}

type User struct {
	XMLName        xml.Name  `xml:"user"`
	ID             int64     `xml:"id,attr"`
	DisplayName    string    `xml:"display_name,attr"`
	AccountCreated time.Time `xml:"account_created,attr"`

	Description string `xml:"description"`
}

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

	body := bytes.NewBuffer([]byte{})
	if err := xml.NewEncoder(body).Encode(data); err != nil {
		return err
	}

	_, err := c.doPlain("PUT", urlPath, body)
	return err
}

type Tag struct {
	XMLName xml.Name `xml:"tag"`
	Key     string   `xml:"k,attr"`
	Value   string   `xml:"v,attr"`
}
