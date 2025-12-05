package Remotehandler

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	Databasehandler "github.com/KevinCFechtel/containermon/handler/database"
	Containermodels "github.com/KevinCFechtel/containermon/models/container"
	Remotemodels "github.com/KevinCFechtel/containermon/models/remotes"
)

type Handler struct {
    DBHandler *Databasehandler.Handler
}

func NewHandler(newDBHandler *Databasehandler.Handler) *Handler {
	return &Handler{
		DBHandler: newDBHandler,
	}
}	

func (h *Handler) GetAndStoreRemoteData(remoteHostConfigs []Remotemodels.RemoteConfig) {
	for _, config := range remoteHostConfigs {
		client := http.Client{
			Timeout: time.Second * 2, // Timeout after 2 seconds
		}
		if !strings.HasPrefix(config.HostAddress, "http://") && !strings.HasPrefix(config.HostAddress, "https://") {
			config.HostAddress = "http://" + config.HostAddress
		}

		req, err := http.NewRequest(http.MethodGet, config.HostAddress + "/json", nil)
		if err != nil {
			log.Println(err)
		}

		if config.HostToken != "" {
			req.Header.Set("Authorization", config.HostToken)
		}

		res, err := client.Do(req)
		if err != nil {
			log.Println("Could not reach remote: " + err.Error())
		}

		if res != nil {
			if res.StatusCode == http.StatusOK {
				if res.Body != nil {
					defer res.Body.Close()
				}

				body, err := io.ReadAll(res.Body)
				if err != nil {
					log.Println(err)
				}

				containers := []Containermodels.Container{}
				err = json.Unmarshal(body, &containers)
				if err != nil {
					log.Println(err)
				}

				for _, container := range containers {
					h.DBHandler.InsertOrUpdateContainer(container, container.Host)
					
				}
				if len(containers) != 0 {
					localContainers, err := h.DBHandler.SelectAllContainers(containers[0].Host)
					if err != nil {
						log.Println("error selecting containers: ", err)
					} else {
						for _, localContainer := range localContainers {
							found := false
							for _, remoteContainer := range containers {
								if localContainer.ID == remoteContainer.ID {
									found = true
								}
							}
							if !found {
								h.DBHandler.DeleteContainer(localContainer.ID)
							}
						}
					}
				}
			}
		}
	}
}



