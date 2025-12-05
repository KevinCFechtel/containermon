package Dockerhandler

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/containrrr/shoutrrr"
	docker_container "github.com/docker/docker/api/types/container"
	docker_client "github.com/docker/docker/client"

	Databasehandler "github.com/KevinCFechtel/containermon/handler/database"
)

type Handler struct {
	DBHandler *Databasehandler.Handler
	socket string
	containerErrorUrl string
	enableDebugging bool
	cache map[string]int
	hostname string
	redBubble string
	greenBubble string
}

func NewHandler(newDBHandler *Databasehandler.Handler, socket string, containerErrorUrl string, enableDebugging bool, hostname string, redBubble string, greenBubble string) *Handler {
	return &Handler{
		DBHandler: newDBHandler,
		socket: socket,
		containerErrorUrl: containerErrorUrl,
		enableDebugging: enableDebugging,
		cache: make(map[string]int),
		hostname: hostname,
		redBubble: redBubble,
		greenBubble: greenBubble,
	}
}

func (h *Handler) DockerHealthCheck() {
	// Connect to Docker socket
	dockerClient, err := docker_client.NewClientWithOpts(
		docker_client.WithHost(h.socket),
		docker_client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		log.Println("Failed to connect to Docker socket: " + err.Error())
		os.Exit(1)
	}
	if h.enableDebugging {
		log.Println("Socket connected")
	}
	// Container list
	containerList, err := dockerClient.ContainerList(context.Background(), docker_container.ListOptions{All: true})
	if err != nil {
		log.Println("Failed to retrieve container list: " + err.Error())
		os.Exit(1)
	}
	if h.enableDebugging {
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
		if h.enableDebugging {
			log.Println("Inspection read for container: " + ctrData.Name)
		}
		// Check for skip label
		inspectContainer := true

		for key, value := range ctrData.Config.Labels {
			if key == "containermon.skip" && value == "true" {
				inspectContainer = false
			}
		}
		if h.enableDebugging {
			log.Println("Container " + ctrData.Name + " inspect enabled: " + fmt.Sprint(inspectContainer))
		}

		if inspectContainer {
			if(ctrData.State.Health != nil) {
				healthstatus := ctrData.State.Health.Status
				if h.enableDebugging {
					log.Println("Health status for container " + ctrData.Name + ": " + healthstatus)
				}
				if healthstatus != "healthy" && healthstatus != "starting" {
					found := h.cache[ctrData.ID]
					if found == 0 {
						// Report unhealthy container via healthcheck URL
						if h.containerErrorUrl != "" {
							err := shoutrrr.Send(h.containerErrorUrl, "<b>ERROR:</b> Container: <b>" + ctrData.Name + "</b> on Host <b>" + h.hostname + "</b> has the health status: <b>" + healthstatus + "</b> " + h.redBubble)
							if err != nil {
								log.Println("Failed to send error log: " + err.Error())
							}
						}
						h.cache[ctrData.ID] = 1
					}
				} else {
					found := h.cache[ctrData.ID]
					if found != 0 {
						// Report unhealthy container via healthcheck URL
						if h.containerErrorUrl != "" {
							err := shoutrrr.Send(h.containerErrorUrl, "<b>RECOVERED:</b> Container: <b>" + ctrData.Name + "</b> on Host <b>" + h.hostname + "</b> has recovered the health status: <b>" + healthstatus + "</b> " + h.greenBubble)
							if err != nil {
								log.Println("Failed to send error log: " + err.Error())
							}
						}
						delete(h.cache, ctrData.ID)
					}
				}
			} else {
				containerStatus := ctrData.State.Status
				if h.enableDebugging {
					log.Println("Container status for container " + ctrData.Name + ": " + containerStatus)
				}
				if containerStatus != "running" {
					found := h.cache[ctrData.ID]
					if found == 0 {
						// Report unhealthy container via healthcheck URL
						if h.containerErrorUrl != "" {
							err := shoutrrr.Send(h.containerErrorUrl, "<b>ERROR:</b> Container: <b>" + ctrData.Name + "</b> on Host <b>" + h.hostname + "</b> has the container status: <b>" + containerStatus + "</b> " + h.redBubble)
							if err != nil {
								log.Println("Failed to send error log: " + err.Error())
							}
						}
						h.cache[ctrData.ID] = 1
					}
				} else {
					found := h.cache[ctrData.ID]
					if found != 0 {
						if h.containerErrorUrl != "" {
							err := shoutrrr.Send(h.containerErrorUrl, "<b>RECOVERED:</b> Container: <b>" + ctrData.Name  + "</b> on Host <b>" + h.hostname + "</b> has recovered with status: <b>" + containerStatus + "</b> " + h.greenBubble)
							if err != nil {
								log.Println("Failed to send error log: " + err.Error())
							}
						}
						delete(h.cache, ctrData.ID)
					}
				}
			}
		}
	}
}
