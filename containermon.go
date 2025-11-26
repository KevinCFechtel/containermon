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
	"github.com/containrrr/shoutrrr"
	docker_container "github.com/docker/docker/api/types/container"
	docker_client "github.com/docker/docker/client"
	"github.com/go-co-op/gocron/v2"
)


func main() {
	var socketPath string
	var hostHealthcheckUrl string
	var containerErrorUrl string
	var cronContainerHealthConfig string
	var cronHostHealthConfig string
	var enableDebugging bool
	var messageOnStartup bool
	greenBubble := "\U0001F7E2"
	redBubble := "\U0001F534"
    
	flag.StringVar(&socketPath, "socketPath", "", "Socket file path for Container engine")
	if(socketPath == "") {
		socketPath = os.Getenv("SOCKET_FILE_PATH")
	}
	flag.StringVar(&hostHealthcheckUrl, "healthcheckUrl", "", "Health check URL to report status")
	if(hostHealthcheckUrl == "") {
		hostHealthcheckUrl = os.Getenv("HOST_HEALTH_CHECK_URL")
	}
	flag.StringVar(&containerErrorUrl, "containerErrorUrl", "", "Error URL to report container errors")
	if(containerErrorUrl == "") {
		containerErrorUrl = os.Getenv("CONTAINER_ERROR_URL")
	}
	flag.StringVar(&cronContainerHealthConfig, "cronContainerHealthConfig", "", "Cron scheduler configuration for Container health check")
	if(cronContainerHealthConfig == "") {
		cronContainerHealthConfig = os.Getenv("CRON_CONTAINER_HEALTH_CONFIG")
	}
	flag.StringVar(&cronHostHealthConfig, "cronHostHealthConfig", "", "Cron scheduler configuration for Host health check")
	if(cronHostHealthConfig == "") {
		cronHostHealthConfig = os.Getenv("CRON_HOST_HEALTH_CONFIG")
	}
	flag.BoolVar(&enableDebugging, "debug", false, "Enable debug logging")
	if os.Getenv("ENABLE_DEBUGGING") != "" {
		if os.Getenv("ENABLE_DEBUGGING") == "true" {
			enableDebugging = true
		} else {
			enableDebugging = false
		}
	}

	flag.BoolVar(&messageOnStartup, "messageOnStartup", false, "Send startup message")
	if os.Getenv("ENABLE_MESSAGE_ON_STARTUP") != "" {
		if os.Getenv("ENABLE_MESSAGE_ON_STARTUP") == "true" {
			messageOnStartup = true
		} else {
			messageOnStartup = false
		}
	}
	socket := ""
	if strings.Contains(socketPath,"podman") {
		socket = "unix:" + socketPath
	} else {
		socket = "unix://" + socketPath
	}

	cache := make(map[string]int)

	if enableDebugging {
		log.Println("Debugging enabled")
		log.Printf("Socket Path: %s\n", socketPath)
		log.Printf("Host healthcheck URL: %s\n", hostHealthcheckUrl)
		log.Printf("Container error URL: %s\n", containerErrorUrl)
		log.Printf("Cron Host health Scheduler Config: %s\n", cronHostHealthConfig)
		log.Printf("Cron Container health Scheduler Config: %s\n", cronContainerHealthConfig)
		log.Printf("Message on Startup %s\n", messageOnStartup)
	}

	hostname, err := os.Hostname()
	if err != nil {
		log.Println("Failed to get Hostname: " + err.Error())
	}

	s, err := gocron.NewScheduler()
	if err != nil {
		log.Println("Failed to start scheduler: " + err.Error())
	}

	defer func() { _ = s.Shutdown() }()

	if cronContainerHealthConfig != "" {
		secondsInContainerCron := false
		if strings.Count(cronContainerHealthConfig, " ") > 4 {
			secondsInContainerCron = true
		}
		defer func() { _ = s.Shutdown() }()
		_, err = s.NewJob(
			gocron.CronJob(
				cronContainerHealthConfig,
				secondsInContainerCron,
			),
			gocron.NewTask(
				func() {
					if enableDebugging {
						log.Println("Starting Cron for Container Health Check")
					}
					
					// HTTP client with timeout
					client := &http.Client{
						Timeout: 10 * time.Second,
					}

					if strings.Contains(socketPath,"podman") {
						if enableDebugging {
							log.Println("Using Podman health check")
						}
						podmanHealthCheck(client, socket, containerErrorUrl, enableDebugging, cache, hostname, redBubble, greenBubble)
					} else {
						if enableDebugging {
							log.Println("Using Docker health check")
						}
						dockerHealthCheck(client, socket, containerErrorUrl, enableDebugging, cache, hostname, redBubble, greenBubble)
					}
				},
			),
		)
		if err != nil {
			log.Println("Failed to config scheduler job: " + err.Error())
		}
	} else {
		log.Println("No Container Health Check cron configuration provided, skipping container health check setup")
	}

	if cronHostHealthConfig != "" {
		secondsInContainerCron := false
		if strings.Count(cronHostHealthConfig, " ") > 4 {
			secondsInContainerCron = true
		}
		_, err = s.NewJob(
			gocron.CronJob(
				cronHostHealthConfig,
				secondsInContainerCron,
			),
			gocron.NewTask(
				func() {
					if enableDebugging {
						log.Println("Starting Cron for Host Health Check")
					}
					
					// HTTP client with timeout
					client := &http.Client{
						Timeout: 10 * time.Second,
					}

					if enableDebugging {
						log.Println("Sending Host Healthcheck")
					}
					resp, err := client.Head(hostHealthcheckUrl)
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
	} else {
		log.Println("No Host Health Check cron configuration provided, skipping host health check setup")
	}
	if messageOnStartup {
		err := shoutrrr.Send(containerErrorUrl, "*STARTUP:* ContainerMon has started successfully on Host *" + hostname + "* " + greenBubble)
		if err != nil {
			log.Println("Failed to send error log: " + err.Error())
		}
	}
	if len(s.Jobs()) > 0 {
		s.Start()
		select {} 
	} else {
		log.Println("No cron jobs configured, exiting")
	}	
}

func podmanHealthCheck(client *http.Client, socket string, containerErrorUrl string, enableDebugging bool, cache map[string]int, hostname string, redBubble string, greenBubble string) {
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
			skipHealthCheck := true
			if(ctrData.State.Health != nil) {
				skipHealthCheck = false
				healthstatus := ctrData.State.Health.Status
				if healthstatus != "" {
					if enableDebugging {
						log.Println("Health status for container " + ctrData.Name + ": " + healthstatus)
					}
					if healthstatus != "healthy" && healthstatus != "starting" {
						found := cache[ctrData.ID]
						if found == 0 {
							// Report unhealthy container via healthcheck URL
							if containerErrorUrl != "" {
							err := shoutrrr.Send(containerErrorUrl, "*ERROR:* Container: *" + ctrData.Name + "* on Host *" + hostname + "* has the health status: *" + healthstatus + "* " + redBubble)
								if err != nil {
									log.Println("Failed to send error log: " + err.Error())
								}
							}
							cache[ctrData.ID] = 1
						}
					} else {
						found := cache[ctrData.ID]
						if found != 0 {
							// Report unhealthy container via healthcheck URL
							if containerErrorUrl != "" {
							err := shoutrrr.Send(containerErrorUrl, "*RECOVERED:* Container: *" + ctrData.Name + "* on Host *" + hostname + "* has recovered the health status: *" + healthstatus + "* " + greenBubble)
								if err != nil {
									log.Println("Failed to send error log: " + err.Error())
								}
							}
							delete(cache,ctrData.ID)
						}
					}
				}
			} 
			if(skipHealthCheck) {
				containerStatus := ctrData.State.Status
				if enableDebugging {
					log.Println("Container status for container " + ctrData.Name + ": " + containerStatus)
				}
				if containerStatus != "running" {
					found := cache[ctrData.ID]
					if found == 0 {
						// Report unhealthy container via healthcheck URL
						if containerErrorUrl != "" {
							err := shoutrrr.Send(containerErrorUrl, "*ERROR:* Container: *" + ctrData.Name + "* on Host *" + hostname + "* has the container status: *" + containerStatus + "* " + redBubble)
							if err != nil {
								log.Println("Failed to send error log: " + err.Error())
							}
						}
						cache[ctrData.ID] = 1
					}
				} else {
					found := cache[ctrData.ID]
					if found != 0 {
						// Report unhealthy container via healthcheck URL
						if containerErrorUrl != "" {
							err := shoutrrr.Send(containerErrorUrl, "*RECOVERED:* Container: *" + ctrData.Name  + "* on Host *" + hostname + "* has recovered with status: *" + containerStatus + "* " + greenBubble)
							if err != nil {
								log.Println("Failed to send error log: " + err.Error())
							}
						}
						delete(cache, ctrData.ID)
					}
				}
			}
		}
	}
}

func dockerHealthCheck(client *http.Client, socket string, containerErrorUrl string, enableDebugging bool, cache map[string]int, hostname string, redBubble string, greenBubble string) {
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
				if healthstatus != "healthy" && healthstatus != "starting" {
					found := cache[ctrData.ID]
					if found == 0 {
						// Report unhealthy container via healthcheck URL
						if containerErrorUrl != "" {
							err := shoutrrr.Send(containerErrorUrl, "*ERROR:* Container: *" + ctrData.Name + "* on Host *" + hostname + "* has the health status: *" + healthstatus + "* " + redBubble)
							if err != nil {
								log.Println("Failed to send error log: " + err.Error())
							}
						}
						cache[ctrData.ID] = 1
					}
				} else {
					found := cache[ctrData.ID]
					if found != 0 {
						// Report unhealthy container via healthcheck URL
						if containerErrorUrl != "" {
							err := shoutrrr.Send(containerErrorUrl, "*RECOVERED:* Container: *" + ctrData.Name + "* on Host *" + hostname + "* has recovered the health status: *" + healthstatus + "* " + greenBubble)
							if err != nil {
								log.Println("Failed to send error log: " + err.Error())
							}
						}
						delete(cache, ctrData.ID)
					}
				}
			} else {
				containerStatus := ctrData.State.Status
				if enableDebugging {
					log.Println("Container status for container " + ctrData.Name + ": " + containerStatus)
				}
				if containerStatus != "running" {
					found := cache[ctrData.ID]
					if found == 0 {
						// Report unhealthy container via healthcheck URL
						if containerErrorUrl != "" {
							err := shoutrrr.Send(containerErrorUrl, "*ERROR:* Container: *" + ctrData.Name + "* on Host *" + hostname + "* has the container status: *" + containerStatus + "* " + redBubble)
							if err != nil {
								log.Println("Failed to send error log: " + err.Error())
							}
						}
						cache[ctrData.ID] = 1
					}
				} else {
					found := cache[ctrData.ID]
					if found != 0 {
						if containerErrorUrl != "" {
							err := shoutrrr.Send(containerErrorUrl, "*RECOVERED:* Container: *" + ctrData.Name  + "* on Host *" + hostname + "* has recovered with status: *" + containerStatus + "* " + greenBubble)
							if err != nil {
								log.Println("Failed to send error log: " + err.Error())
							}
						}
						delete(cache, ctrData.ID)
					}
				}
			}
		}
	}
}