package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/fatih/structs"
	"github.com/gorilla/mux"
	"github.com/maliceio/go-plugin-utils/database/elasticsearch"
	"github.com/maliceio/go-plugin-utils/utils"
	"github.com/maliceio/malice/utils/clitable"
	"github.com/parnurzeal/gorequest"
	"github.com/urfave/cli"
)

// Version stores the plugin's version
var Version string

// BuildTime stores the plugin's build time
var BuildTime string

var path string

const (
	name     = "avast"
	category = "av"
)

type pluginResults struct {
	ID   string      `json:"id" gorethink:"id,omitempty"`
	Data ResultsData `json:"avast" gorethink:"avast"`
}

// Avast json object
type Avast struct {
	Results ResultsData `json:"avast"`
}

// ResultsData json object
type ResultsData struct {
	Infected bool   `json:"infected" gorethink:"infected"`
	Result   string `json:"result" gorethink:"result"`
	Engine   string `json:"engine" gorethink:"engine"`
	Database string `json:"database" gorethink:"database"`
	Updated  string `json:"updated" gorethink:"updated"`
}

func assert(err error) {
	if err != nil {
		log.WithFields(log.Fields{
			"plugin":   name,
			"category": category,
			"path":     path,
		}).Fatal(err)
	}
}

// AvScan performs antivirus scan
func AvScan(timeout int) Avast {

	// Give avastd 10 seconds to finish
	avastdCtx, avastdCancel := context.WithTimeout(context.Background(), time.Duration(30)*time.Second)
	defer avastdCancel()
	// Avast needs to have the daemon started first
	_, err := utils.RunCommand(avastdCtx, "/etc/init.d/avast", "start")
	assert(err)

	var results ResultsData

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	output, err := utils.RunCommand(ctx, "scan", "-abfu", path)
	assert(err)
	results, err = ParseAvastOutput(output)

	if err != nil {
		// If fails try a second time
		output, err := utils.RunCommand(ctx, "scan", "-abfu", path)
		assert(err)
		results, err = ParseAvastOutput(output)
		assert(err)
	}

	return Avast{
		Results: results,
	}
}

// ParseAvastOutput convert avast output into ResultsData struct
func ParseAvastOutput(avastout string) (ResultsData, error) {

	log.WithFields(log.Fields{
		"plugin":   name,
		"category": category,
		"path":     path,
	}).Debug("Avast Output: ", avastout)

	avast := ResultsData{
		Infected: false,
		Engine:   getAvastVersion(),
		Database: getAvastVPS(),
		Updated:  getUpdatedDate(),
	}

	result := strings.Split(avastout, "\t")

	if !strings.Contains(avastout, "[OK]") {
		avast.Infected = true
		avast.Result = strings.TrimSpace(result[1])
	}

	return avast, nil
}

// Get Anti-Virus scanner version
func getAvastVersion() string {
	versionOut, err := utils.RunCommand(nil, "/bin/scan", "-v")
	assert(err)
	log.Debug("Avast Version: ", versionOut)
	return strings.TrimSpace(versionOut)
}

func getAvastVPS() string {
	versionOut, err := utils.RunCommand(nil, "/bin/scan", "-V")
	assert(err)
	log.Debug("Avast Database: ", versionOut)
	return strings.TrimSpace(versionOut)
}

func parseUpdatedDate(date string) string {
	layout := "Mon, 02 Jan 2006 15:04:05 +0000"
	t, _ := time.Parse(layout, date)
	return fmt.Sprintf("%d%02d%02d", t.Year(), t.Month(), t.Day())
}

func getUpdatedDate() string {
	if _, err := os.Stat("/opt/malice/UPDATED"); os.IsNotExist(err) {
		return BuildTime
	}
	updated, err := ioutil.ReadFile("/opt/malice/UPDATED")
	assert(err)
	return string(updated)
}

func updateAV(ctx context.Context) error {
	fmt.Println("Updating Avast...")
	// Avast needs to have the daemon started first
	exec.Command("/etc/init.d/avast", "start").Output()

	fmt.Println(utils.RunCommand(ctx, "/var/lib/avast/Setup/avast.vpsupdate"))
	// Update UPDATED file
	t := time.Now().Format("20060102")
	err := ioutil.WriteFile("/opt/malice/UPDATED", []byte(t), 0644)
	return err
}

func printMarkDownTable(avast Avast) {

	fmt.Println("#### Avast")
	table := clitable.New([]string{"Infected", "Result", "Engine", "Updated"})
	table.AddRow(map[string]interface{}{
		"Infected": avast.Results.Infected,
		"Result":   avast.Results.Result,
		"Engine":   avast.Results.Engine,
		"Updated":  avast.Results.Updated,
	})
	table.Markdown = true
	table.Print()
}

func printStatus(resp gorequest.Response, body string, errs []error) {
	fmt.Println(body)
}

func webService() {
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/scan", webAvScan).Methods("POST")
	log.WithFields(log.Fields{
		"plugin":   name,
		"category": category,
	}).Info("web service listening on port :3993")
	log.Fatal(http.ListenAndServe(":3993", router))
}

func webAvScan(w http.ResponseWriter, r *http.Request) {

	r.ParseMultipartForm(32 << 20)
	file, header, err := r.FormFile("malware")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, "Please supply a valid file to scan.")
		log.WithFields(log.Fields{
			"plugin":   name,
			"category": category,
		}).Error(err)
	}
	defer file.Close()

	log.WithFields(log.Fields{
		"plugin":   name,
		"category": category,
	}).Debug("Uploaded fileName: ", header.Filename)

	tmpfile, err := ioutil.TempFile("/malware", "web_")
	assert(err)
	defer os.Remove(tmpfile.Name()) // clean up

	data, err := ioutil.ReadAll(file)

	if _, err = tmpfile.Write(data); err != nil {
		assert(err)
	}
	if err = tmpfile.Close(); err != nil {
		assert(err)
	}

	// Do AV scan
	path = tmpfile.Name()
	avast := AvScan(60)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(avast); err != nil {
		assert(err)
	}
}

func main() {
	var elastic string

	cli.AppHelpTemplate = utils.AppHelpTemplate
	app := cli.NewApp()

	app.Name = "avast"
	app.Author = "blacktop"
	app.Email = "https://github.com/blacktop"
	app.Version = Version + ", BuildTime: " + BuildTime
	app.Compiled, _ = time.Parse("20060102", BuildTime)
	app.Usage = "Malice Avast AntiVirus Plugin"
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "verbose, V",
			Usage: "verbose output",
		},
		cli.StringFlag{
			Name:        "elasitcsearch",
			Value:       "",
			Usage:       "elasitcsearch address for Malice to store results",
			EnvVar:      "MALICE_ELASTICSEARCH",
			Destination: &elastic,
		},
		cli.BoolFlag{
			Name:  "table, t",
			Usage: "output as Markdown table",
		},
		cli.BoolFlag{
			Name:   "callback, c",
			Usage:  "POST results back to Malice webhook",
			EnvVar: "MALICE_ENDPOINT",
		},
		cli.BoolFlag{
			Name:   "proxy, x",
			Usage:  "proxy settings for Malice webhook endpoint",
			EnvVar: "MALICE_PROXY",
		},
		cli.IntFlag{
			Name:   "timeout",
			Value:  120,
			Usage:  "malice plugin timeout (in seconds)",
			EnvVar: "MALICE_TIMEOUT",
		},
	}
	app.Commands = []cli.Command{
		{
			Name:    "update",
			Aliases: []string{"u"},
			Usage:   "Update virus definitions",
			Action: func(c *cli.Context) error {
				return updateAV(nil)
			},
		},
		{
			Name:  "web",
			Usage: "Create a Avast scan web service",
			Action: func(c *cli.Context) error {
				webService()
				return nil
			},
		},
	}
	app.Action = func(c *cli.Context) error {
		var err error

		if c.Bool("verbose") {
			log.SetLevel(log.DebugLevel)
		}

		if c.Args().Present() {
			path, err = filepath.Abs(c.Args().First())
			assert(err)

			if _, err = os.Stat(path); os.IsNotExist(err) {
				assert(err)
			}

			avast := AvScan(c.Int("timeout"))

			// upsert into Database
			elasticsearch.InitElasticSearch(elastic)
			elasticsearch.WritePluginResultsToDatabase(elasticsearch.PluginResults{
				ID:       utils.Getopt("MALICE_SCANID", utils.GetSHA256(path)),
				Name:     name,
				Category: category,
				Data:     structs.Map(avast.Results),
			})

			if c.Bool("table") {
				printMarkDownTable(avast)
			} else {
				avastJSON, err := json.Marshal(avast)
				assert(err)
				if c.Bool("callback") {
					request := gorequest.New()
					if c.Bool("proxy") {
						request = gorequest.New().Proxy(os.Getenv("MALICE_PROXY"))
					}
					request.Post(os.Getenv("MALICE_ENDPOINT")).
						Set("X-Malice-ID", utils.Getopt("MALICE_SCANID", utils.GetSHA256(path))).
						Send(string(avastJSON)).
						End(printStatus)

					return nil
				}
				fmt.Println(string(avastJSON))
			}
		} else {
			log.WithFields(log.Fields{
				"plugin":   name,
				"category": category,
			}).Fatal(fmt.Errorf("Please supply a file to scan with malice/avast"))
		}
		return nil
	}

	err := app.Run(os.Args)
	assert(err)
}
