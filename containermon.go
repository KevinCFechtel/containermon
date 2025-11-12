package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/containers/podman/v5/pkg/bindings"
	"github.com/containers/podman/v5/pkg/bindings/containers"
	"github.com/go-co-op/gocron/v2"
)


func main() {
	var socketPath string
	var healthcheckUrl string
	var cronScheduleConfig string

	flag.StringVar(&healthcheckUrl, "healthcheckUrl", "", "Healthcheck URL for reporting container status")
	flag.StringVar(&socketPath, "socketPath", "/var/run/containermon.sock", "File Path to container socket")
	flag.StringVar(&cronScheduleConfig, "cronScheduleConfig", "* * * * *", "Cron schedule config for checking container status")

	socket := "unix:" + socketPath

	s, err := gocron.NewScheduler()
	if err != nil {
		log.Println("Failed to start scheduler: " + err.Error())
	}
	defer func() { _ = s.Shutdown() }()
	_, err = s.NewJob(
		gocron.CronJob(
			cronScheduleConfig,
			false,
		),
		gocron.NewTask(
			func() {
				client := &http.Client{
					Timeout: 10 * time.Second,
				}
				// Connect to Podman socket
				connText, err := bindings.NewConnection(context.Background(), socket)
				if err != nil {
						fmt.Println(err)
						os.Exit(1)
				}

				// Container list
				containerLatestList, err := containers.List(connText, nil)
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}

				for _, r := range containerLatestList {
					// Container inspect
					ctrData, err := containers.Inspect(connText, r.ID, nil)
					if err != nil {
							fmt.Println(err)
							os.Exit(1)
					}
					inspectContainer := true

					for key, value := range ctrData.Config.Labels {
						if key == "containermon.skip" && value == "true" {
							inspectContainer = false
						}
					}
					if inspectContainer {
						if ctrData.State.Health != nil {
							healthstatus := ctrData.State.Health.Status
							if healthstatus != "healthy" {
								// Report unhealthy container via healthcheck URL
								if healthcheckUrl != "" {
									resp, err := client.Post(healthcheckUrl + "/fail", "text/plain; charset=utf-8",  strings.NewReader(ctrData.Name))
									if err != nil {
										log.Println("Failed to send error log: " + err.Error())
									}
									if resp.StatusCode != 200 {
										log.Println("Failed to send error log, response code: " + resp.Status)
									}
								}
							}
						} else {
							containerStatus := ctrData.State.Status
							if containerStatus != "running" {
								// Report non-running container via healthcheck URL
								if healthcheckUrl != "" {
									resp, err := client.Post(healthcheckUrl + "/fail", "text/plain; charset=utf-8",  strings.NewReader(ctrData.Name))
									if err != nil {
										log.Println("Failed to send error log: " + err.Error())
									}
									if resp.StatusCode != 200 {
										log.Println("Failed to send error log, response code: " + resp.Status)
									}
								}
							}
						}
					}
				}

				resp, err := client.Get(healthcheckUrl)
				if err != nil {
					log.Println("Failed to send success log: " + err.Error())
				}
				if resp.StatusCode != 200 {
					log.Println("Failed to send success log, response code: " + resp.Status)
				}
			},
		),
	)
	if err != nil {
		log.Println("Failed to config scheduler job: " + err.Error())
	}
	s.Start()
	select {} // wait forever
}