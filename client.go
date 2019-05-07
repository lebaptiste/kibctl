package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Logger is the interface used to report diagnostic details
type Logger interface {
	Printf(format string, v ...interface{})
}

type client struct {
	Host     string
	Username string
	Password string
	Logger
}

func (c *client) _import(payload []byte) error {
	c.Logger.Printf("importing dashboard:\n%v\n", string(payload))
	u := fmt.Sprintf(`%v/api/kibana/dashboards/import?force=true`, c.Host)
	req, err := http.NewRequest("POST", u, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("kbn-xsrf", "true")
	req.SetBasicAuth(c.Username, c.Password)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	details, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("failed to import dashboard. Status:%v. Response:%v.\n", resp.Status, string(details))
	}
	c.Logger.Printf("SUCCESS\n%v\n", string(details))
	return nil
}

func (c *client) export(name string) ([]byte, error) {
	c.Logger.Printf("searching dashboards matching name %v\n", name)
	result, err := c.searchDashboard(fmt.Sprintf(`"%v"`, name))
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, errors.Errorf("no dashboard found matching: %v.\n", name)
	}
	if len(result) > 1 {
		return nil, errors.Errorf("more than one dashboard found matching: %v.\n", name)
	}
	c.Logger.Printf("found dashboard id %v", result[0].ID)

	c.Logger.Printf("retrieving partial dashboard export from api...\n")
	dashboard, err := c.getDashboard(result[0].ID)
	if err != nil {
		return nil, err
	}

	indiceNames, err := c.scanForIndexPatterns(dashboard)
	if err != nil {
		return nil, err
	}

	for _, name := range indiceNames {
		indexPattern, err := c.getIndexPattern(name)
		if err != nil {
			return nil, err
		}
		c.Logger.Printf("adding index-template %v", name)
		//element order does not matter
		dashboard, err = sjson.SetRawBytes(dashboard, "objects.-1", indexPattern)

	}

	return dashboard, nil
}

type dashboard struct {
	ID         string     `json:"id"`
	Attributes attributes `json:"attributes"`
}

type attributes struct {
	Title string `json:"title"`
}

func (c *client) searchDashboard(pattern string) ([]dashboard, error) {
	u := fmt.Sprintf(`%v/api/saved_objects/_find?type=dashboard&per_page=200&search_fields=title&search=%v`, c.Host, pattern)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.Username, c.Password)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		details, _ := ioutil.ReadAll(resp.Body)
		return nil, errors.Errorf("failed to search dashboard name %v. Status:%v. Response:%v.\n", pattern, resp.Status, string(details))
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var dashboards []dashboard
	for _, value := range gjson.Get(string(body), "saved_objects").Array() {
		var d dashboard
		err := json.Unmarshal([]byte(value.String()), &d)
		if err != nil {
			return nil, errors.Wrapf(err, "could not parse dashboard definition")
		}
		dashboards = append(dashboards, d)
	}

	return dashboards, nil
}

func (c *client) getDashboard(id string) ([]byte, error) {
	u := fmt.Sprintf("%v/api/kibana/dashboards/export?dashboard=%v", c.Host, id)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(c.Username, c.Password)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		details, _ := ioutil.ReadAll(resp.Body)
		return nil, errors.Errorf("failed to retrieve dashboard id %v. Status:%v. Response:%v.\n", id, resp.Status, string(details))
	}

	return ioutil.ReadAll(resp.Body)
}

func (c *client) scanForIndexPatterns(dashboard []byte) ([]string, error) {
	names := make(map[string]struct{})
	// scan all visualisations
	for _, val := range gjson.Get(string(dashboard), "objects.#.attributes.visState").Array() {
		visualisation := strings.Replace(val.String(), `\"`, `"`, -1)
		index := gjson.Get(visualisation, "params.index_pattern")
		if index.Exists() {
			names[index.String()] = struct{}{}
		}
	}

	list := make([]string, 0, len(names))
	for key := range names {
		list = append(list, key)
	}

	return list, nil
}

func (c *client) getIndexPattern(name string) ([]byte, error) {
	u := fmt.Sprintf(`%v/api/saved_objects/_find?type=index-pattern&search_fields=title&search="%v"`, c.Host, name)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.Username, c.Password)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		details, _ := ioutil.ReadAll(resp.Body)
		return nil, errors.Errorf("failed to retrieve index-pattern title %v. Status:%v. Response: %v.\n", name, resp.Status, string(details))
	}

	json, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	patterns := gjson.Get(string(json), "saved_objects").Array()
	if len(patterns) == 0 {
		return nil, errors.Errorf("no index-pattern found matching: %v.\n", name)
	}
	if len(patterns) > 1 {
		return nil, errors.Errorf("More than one index-pattern found matching: %v.\n", name)
	}

	return []byte(patterns[0].String()), nil
}
