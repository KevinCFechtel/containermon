package Podmanhandler

import (
	"context"
	"fmt"
	"log"
	"os"

	podman_binding "github.com/containers/podman/v5/pkg/bindings"
	podman_containers "github.com/containers/podman/v5/pkg/bindings/containers"
	podman_images "github.com/containers/podman/v5/pkg/bindings/images"
	"github.com/containrrr/shoutrrr"

	Databasehandler "github.com/KevinCFechtel/containermon/handler/database"
	Containermodels "github.com/KevinCFechtel/containermon/models/container"
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


func (h *Handler) PodmanHealthCheck() {
	// Connect to Podman socket
	connText, err := podman_binding.NewConnection(context.Background(), h.socket)
	if err != nil {
			log.Println(err)
			os.Exit(1)
	}
	if h.enableDebugging {
		log.Println("Socket connected")
	}
	// Container list
	options := podman_containers.ListOptions{
	}
	options.WithAll(true)
	containerLatestList, err := podman_containers.List(connText, &options)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	if h.enableDebugging {
		log.Println("Container list retrieved, total containers: " + fmt.Sprint(len(containerLatestList)))
	}

	// Image list
	image_options := podman_images.ListOptions{
	}
	imageList, err := podman_images.List(connText, &image_options)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	// Process each container
	for _, r := range containerLatestList {
		// Inspect each container
		ctrData, err := podman_containers.Inspect(connText, r.ID, nil)
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}
		if h.enableDebugging {
			log.Println("Inspection read for container: " + ctrData.Name)
		}
		imageDigest := ""
		for _, img := range imageList {
			if img.ID == ctrData.Image {
				if h.enableDebugging {
					log.Println("Image found for container " + ctrData.Name + ": " + img.RepoTags[0])
				}
				imageDigest = img.Digest
			}
		}
		containerStatus := ""
		if ctrData.State.Health != nil {
			containerStatus = ctrData.State.Health.Status
		} else {
			containerStatus = ctrData.State.Status
		}
		container := Containermodels.Container{
			ID:        	ctrData.ID,
			Name:      	ctrData.Name,
			Status:    	containerStatus,
			ImageName: 	ctrData.Config.Image,
			ImageDigest: 	imageDigest,
		}
		h.DBHandler.InsertOrUpdateContainer(container, h.hostname)

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
			skipHealthCheck := true
			if(ctrData.State.Health != nil) {
				skipHealthCheck = false
				healthstatus := ctrData.State.Health.Status
				if healthstatus != "" {
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
							delete(h.cache,ctrData.ID)
						}
					}
				}
			} 
			if(skipHealthCheck) {
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
						// Report unhealthy container via healthcheck URL
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
	localContainers, err := h.DBHandler.SelectAllContainers(h.hostname)
	if err != nil {
		log.Println("error selecting containers: ", err)
	} else {
		for _, localContainer := range localContainers {
			found := false
			for _, latestContainer := range containerLatestList {
				if localContainer.ID == latestContainer.ID {
					found = true
				}
			}
			if !found {
				h.DBHandler.DeleteContainer(localContainer.ID)
			}
		}
	}
}