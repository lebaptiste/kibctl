package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

var verbose bool
var host, username, password string

type cmdLogger struct {
	IsVerbose bool
	*log.Logger
}

func (c *cmdLogger) Printf(format string, v ...interface{}) {
	if c.IsVerbose {
		c.Logger.Printf(format, v...)
	}
}

func main() {

	app := cli.NewApp()
	app.Name = "kibctl"
	app.Usage = "kibctl is a cli tool for kibana"
	cli.VersionFlag = cli.BoolFlag{Name: "version"}
	cli.HelpFlag = cli.BoolFlag{Name: "help"}

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:        "verbose, v",
			Usage:       "provide additional details",
			Destination: &verbose,
		},
		cli.StringFlag{
			Name:        "host, h",
			Usage:       "Kibana api endpoint (required)",
			Destination: &host,
			EnvVar:      "KIBANA_HOST",
		},
		cli.StringFlag{
			Name:        "username, u",
			Usage:       "Basic auth username (required)",
			Destination: &username,
			EnvVar:      "KIBANA_USERNAME",
		},
		cli.StringFlag{
			Name:        "password, p",
			Usage:       "Basic auth password (required)",
			Destination: &password,
			EnvVar:      "KIBANA_PASSWORD",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:  "dashboard",
			Usage: "option for dashbaord",
			Subcommands: []cli.Command{
				{
					Name:   "import",
					Usage:  "import PAYLOAD - import the dashboard definition",
					Action: _import,
				},
				{
					Name:   "export",
					Usage:  "export NAME - export a json including the visualisation and index-template dependencies",
					Action: export,
				},
				{
					Name:   "list",
					Usage:  "list PATTERN - list dashboards with title matching the pattern",
					Action: list,
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func newClient() *client {
	return &client{
		Host:     host,
		Username: username,
		Password: password,
		Logger: &cmdLogger{
			Logger:    log.New(os.Stdout, "", log.LstdFlags),
			IsVerbose: verbose,
		},
	}
}

func checkGlobals(c *cli.Context) error {
	if host == "" {
		return cli.NewExitError("kibana host not defined", 1)
	}
	if username == "" {
		return cli.NewExitError("kibana username not defined", 1)
	}
	if password == "" {
		return cli.NewExitError("kibana password not defined", 1)
	}
	return nil
}

func _import(c *cli.Context) error {
	bytes, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return cli.NewExitError(errors.Wrap(err, "could not read import input"), 2)
	}
	err = newClient()._import(bytes)
	if err != nil {
		return cli.NewExitError(err, 2)
	}
	return nil
}

func export(c *cli.Context) error {
	if err := checkGlobals(c); err != nil {
		return err
	}
	name := c.Args().First()
	if name == "" {
		return cli.NewExitError("dashboard name missing", 1)
	}
	dashboard, err := newClient().export(name)
	if err != nil {
		return cli.NewExitError(err, 2)
	}
	os.Stdout.Write(dashboard)
	return nil
}

func list(c *cli.Context) error {
	if err := checkGlobals(c); err != nil {
		return err
	}
	pattern := c.Args().First()
	dashboards, err := newClient().searchDashboard(pattern)
	if err != nil {
		return cli.NewExitError(err, 2)
	}
	os.Stdout.WriteString(fmt.Sprintf("%-40v %v\n", "ID", "NAME"))
	for _, val := range dashboards {
		os.Stdout.WriteString(fmt.Sprintf("%-40v %v\n", val.ID, val.Attributes.Title))
	}
	return nil
}
