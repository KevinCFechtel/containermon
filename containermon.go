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

	podman_binding "github.com/containers/podman/v5/pkg/bindings"
	podman_containers "github.com/containers/podman/v5/pkg/bindings/containers"
	docker_container "github.com/docker/docker/api/types/container"
	docker_client "github.com/docker/docker/client"
	"github.com/go-co-op/gocron/v2"
)


func main() {
	var socketPath string
	var healthcheckUrl string
	var cronSchedulerConfig string
	var enableDebugging bool
    
	flag.StringVar(&socketPath, "socketPath", "", "Socket file path for Container engine")
	if(socketPath == "") {
		socketPath = os.Getenv("SOCKET_FILE_PATH")
	}
	flag.StringVar(&healthcheckUrl, "healthcheckUrl", "", "Health check URL to report status")
	if(healthcheckUrl == "") {
		healthcheckUrl = os.Getenv("HEALTH_CHECK_URL")
	}
	flag.StringVar(&cronSchedulerConfig, "cronSchedulerConfig", "", "Cron scheduler configuration string")
	if(cronSchedulerConfig == "") {
		cronSchedulerConfig = os.Getenv("CRON_SCHEDULER_CONFIG")
	}
	flag.BoolVar(&enableDebugging, "debug", false, "Enable debug logging")
	if os.Getenv("ENABLE_DEBUGGING") != "" {
		if os.Getenv("ENABLE_DEBUGGING") == "true" {
			enableDebugging = true
		} else {
			enableDebugging = false
		}
	}
	socket := ""
	if strings.Contains(socketPath,"podman") {
		socket = "unix:" + socketPath
	} else {
		socket = "unix://" + socketPath
	}

	if enableDebugging {
		log.Println("Debugging enabled")
		log.Printf("Socket Path: %s\n", socketPath)
		log.Printf("Healthcheck URL: %s\n", healthcheckUrl)
		log.Printf("Cron Scheduler Config: %s\n", cronSchedulerConfig)
	}

	s, err := gocron.NewScheduler()
	if err != nil {
		log.Println("Failed to start scheduler: " + err.Error())
	}
	if(cronSchedulerConfig != "") {
		defer func() { _ = s.Shutdown() }()
		_, err = s.NewJob(
			gocron.CronJob(
				cronSchedulerConfig,
				false,
			),
			gocron.NewTask(
				func() {
					if enableDebugging {
						log.Println("Starting Cron")
					}
					
					// HTTP client with timeout
					client := &http.Client{
						Timeout: 10 * time.Second,
					}

					foundError := false
					if strings.Contains(socketPath,"podman") {
						if enableDebugging {
							log.Println("Using Podman health check")
						}
						foundError = podmanHealthCheck(client, socket, healthcheckUrl, enableDebugging)
					} else {
						if enableDebugging {
							log.Println("Using Docker health check")
						}
						foundError = dockerHealthCheck(client, socket, healthcheckUrl, enableDebugging)
					}
					// If no errors found, send success log
					if !foundError {
						if enableDebugging {
							log.Println("All containers processed, sending success log")
						}
						resp, err := client.Get(healthcheckUrl)
						if err != nil {
							log.Println("Failed to send success log: " + err.Error())
						}
						if resp.StatusCode != 200 {
							log.Println("Failed to send success log, response code: " + resp.Status)
						}
					}
					
				},
			),
		)
		if err != nil {
			log.Println("Failed to config scheduler job: " + err.Error())
		}
		s.Start()
		select {} // wait forever
	} else {
		log.Println("No cron schedule config provided, exiting...")
	}	
}

func podmanHealthCheck(client *http.Client, socket string, healthcheckUrl string, enableDebugging bool) bool {
	foundError := false
	// Connect to Podman socket
	connText, err := podman_binding.NewConnection(context.Background(), socket)
	if err != nil {
			fmt.Println(err)
			os.Exit(1)
	}
	if enableDebugging {
		log.Println("Socket connected")
	}
	// Container list
	options := podman_containers.ListOptions{
	}
	options.WithAll(true)
	containerLatestList, err := podman_containers.List(connText, &options)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if enableDebugging {
		log.Println("Container list retrieved, total containers: " + fmt.Sprint(len(containerLatestList)))
	}
	
	// Process each container
	for _, r := range containerLatestList {
		// Inspect each container
		ctrData, err := podman_containers.Inspect(connText, r.ID, nil)
		if err != nil {
				fmt.Println(err)
				os.Exit(1)
		}
		if enableDebugging {
			log.Println("Inspection read for container: " + ctrData.Name)
		}
		// Check for skip label
		inspectContainer := true

		for key, value := range ctrData.Config.Labels {
			if key == "containermon.skip" && value == "true" {
				inspectContainer = false
			}
		}
		if enableDebugging {
			log.Println("Container " + ctrData.Name + " inspect enabled: " + fmt.Sprint(inspectContainer))
		}

		if inspectContainer {
			healthstatus := ctrData.State.Health.Status
			if healthstatus != "" {
				if enableDebugging {
					log.Println("Health status for container " + ctrData.Name + ": " + healthstatus)
				}
				if healthstatus != "healthy" {
					foundError = true
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
				if enableDebugging {
					log.Println("Container status for container " + ctrData.Name + ": " + containerStatus)
				}
				if containerStatus != "running" {
					foundError = true
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
	return foundError
}

func dockerHealthCheck(client *http.Client, socket string, healthcheckUrl string, enableDebugging bool) bool {
	foundError := false
	// Connect to Docker socket
	dockerClient, err := docker_client.NewClientWithOpts(
		docker_client.WithHost(socket),
		docker_client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		log.Println("Failed to connect to Docker socket: " + err.Error())
		os.Exit(1)
	}
	if enableDebugging {
		log.Println("Socket connected")
	}
	// Container list
	containerList, err := dockerClient.ContainerList(context.Background(), docker_container.ListOptions{All: true})
	if err != nil {
		log.Println("Failed to retrieve container list: " + err.Error())
		os.Exit(1)
	}
	if enableDebugging {
		log.Println("Container list retrieved, total containers: " + fmt.Sprint(len(containerList)))
	}

	// Process each container
	for _, r := range containerList {
		// Inspect each container
		ctrData, err := dockerClient.ContainerInspect(context.Background(), r.ID)
		if err != nil {
			log.Println("Failed to inspect container " + r.ID + ": " + err.Error())
			os.Exit(1)
		}
		if enableDebugging {
			log.Println("Inspection read for container: " + ctrData.Name)
		}
		// Check for skip label
		inspectContainer := true

		for key, value := range ctrData.Config.Labels {
			if key == "containermon.skip" && value == "true" {
				inspectContainer = false
			}
		}
		if enableDebugging {
			log.Println("Container " + ctrData.Name + " inspect enabled: " + fmt.Sprint(inspectContainer))
		}

		if inspectContainer {
			if(ctrData.State.Health != nil) {
				healthstatus := ctrData.State.Health.Status
				if enableDebugging {
					log.Println("Health status for container " + ctrData.Name + ": " + healthstatus)
				}
				if healthstatus != "healthy" {
					foundError = true
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
				if enableDebugging {
					log.Println("Container status for container " + ctrData.Name + ": " + containerStatus)
				}
				if containerStatus != "running" {
					foundError = true
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
	return foundError
}