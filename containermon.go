package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"database/sql"

	podman_binding "github.com/containers/podman/v5/pkg/bindings"
	podman_containers "github.com/containers/podman/v5/pkg/bindings/containers"
	podman_images "github.com/containers/podman/v5/pkg/bindings/images"
	"github.com/containrrr/shoutrrr"
	docker_container "github.com/docker/docker/api/types/container"
	docker_client "github.com/docker/docker/client"
	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type Container struct {
    ID 				string
	Host 			string
    Name     		string
	Status 			string
	ImageName 		string
	ImageDigest 	string
	ImageDigestNew 	string
}

type ContainerWeb struct {
    ID 				string
	Host 			string
    Name     		string
	Status 			string
	ImageName 		string
	ImageDigest 	string
	ImageDigestNew 	string
	EnableDiunWebhook bool
}

type DuinWebHookBody struct {
    Hostname 	string
	Status 		string
    Provider    string
	Image 		string
	Digest 		string
	Created 	string
	Platform 	string
}

type Handler struct {
    DB *sql.DB
	DiunWebhookEnabled bool
	AgentToken string
	DiunWebhookToken string
	Sessions map[string]session
	webUIPassword string
}

type ContainerPageData struct {
    PageTitle 	string
	DiunWebhookEnabled bool
    Containers 	[]ContainerWeb
}

type RemoteConfig struct {
	HostAddress string
	HostToken   string
}

type session struct {
	expiry   time.Time
}


func main() {
	var socketPath string
	var hostHealthcheckUrl string
	var containerErrorUrl string
	var agentToken string
	var diunWebhookToken string
	var cronContainerHealthConfig string
	var cronHostHealthConfig string
	var cronRemoteConfig string
	var enableDebugging bool
	var messageOnStartup bool
	var enableDiunWebhook bool
	var dbPath string
	var webUIPassword string
	var sessions = map[string]session{}
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
	flag.StringVar(&agentToken, "agentToken", "", "Token for agent authentication")
	if(agentToken == "") {
		agentToken = os.Getenv("AGENT_TOKEN")
	}
	flag.StringVar(&diunWebhookToken, "diunWebhookToken", "", "Token for agent authentication")
	if(diunWebhookToken == "") {
		diunWebhookToken = os.Getenv("DIUN_WEBHOOK_TOKEN")
	}
	flag.StringVar(&webUIPassword, "webUIPassword", "", "Password for Web UI access")
	if(webUIPassword == "") {
		webUIPassword = os.Getenv("WEBUI_PASSWORD")
	}
	remoteHostConfigs := []RemoteConfig{}
	for _, e := range os.Environ() {
		if i := strings.Index(e, "="); i >= 0 {
			if strings.HasPrefix(e, "REMOTE_CONFIG_HOST_") {
				configCoutnter := strings.TrimPrefix(e[:i], "REMOTE_CONFIG_HOST_")
				remoteToken := os.Getenv("REMOTE_CONFIG_TOKEN_" + configCoutnter)
				remoteHostConfigs = append(remoteHostConfigs, RemoteConfig{ 
					HostAddress: e[i+1:],
					HostToken: remoteToken,
				})
			}
		}
	}

	flag.StringVar(&cronRemoteConfig, "cronRemoteConfig", "", "Cron scheduler configuration for Remote data fetch")
	if(cronRemoteConfig == "") {
		cronRemoteConfig = os.Getenv("CRON_REMOTE_CONFIG")
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
	flag.StringVar(&dbPath, "dbPath", "", "Path to Sqlite DB file")
	if(dbPath == "") {
		dbPath = os.Getenv("DB_PATH")
	}
	flag.BoolVar(&enableDiunWebhook, "enableDiunWebhook", false, "Enable debug logging")
	if os.Getenv("ENABLE_DIUN_WEBHOOK") != "" {
		if os.Getenv("ENABLE_DIUN_WEBHOOK") == "true" {
			enableDiunWebhook = true
		} else {
			enableDiunWebhook = false
		}
	}
	os.Environ()

	socket := ""
	if strings.Contains(socketPath,"podman") {
		socket = "unix:" + socketPath
	} else {
		socket = "unix://" + socketPath
	}

	cache := make(map[string]int)

	db, _ := sql.Open("sqlite", "file:"+dbPath)
	sqlCreateTable := `
	CREATE TABLE IF NOT EXISTS containers (
		ID TEXT PRIMARY KEY,
		Host TEST,
		Name TEXT,
		Status TEXT,
		ImageName TEXT,
		ImageDigest TEXT
	);`
	_, err := db.Exec(sqlCreateTable)
	if err != nil {
		log.Println("error creating containers table: ", err)
	}
	defer db.Close()
	sqlCreateTable = `
	CREATE TABLE IF NOT EXISTS diun (
		ImageName TEXT PRIMARY KEY,
		ImageDigest TEXT
	);`
	_, err = db.Exec(sqlCreateTable)
	if err != nil {
		log.Println("error creating containers table: ", err)
	}

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
						podmanHealthCheck(client, socket, containerErrorUrl, enableDebugging, cache, hostname, redBubble, greenBubble, db)
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
	if cronRemoteConfig != "" {
		secondsInRemoteCron := false
		if strings.Count(cronRemoteConfig, " ") > 4 {
			secondsInRemoteCron = true
		}
		defer func() { _ = s.Shutdown() }()
		_, err = s.NewJob(
			gocron.CronJob(
				cronRemoteConfig,
				secondsInRemoteCron,
			),
			gocron.NewTask(
				func() {
					if enableDebugging {
						log.Println("Starting Cron for Remote Data Fetch")
					}
					
					getAndStoreRemoteData(remoteHostConfigs, db)
				},
			),
		)
		if err != nil {
			log.Println("Failed to config scheduler job: " + err.Error())
		}
	} else {
		log.Println("No Remote Data Fetch cron configuration provided, skipping remote data fetch setup")
	}
	if messageOnStartup {
		err := shoutrrr.Send(containerErrorUrl, "<b>STARTUP:</b> ContainerMon has started successfully on Host <b>" + hostname + "</b> " + greenBubble)
		if err != nil {
			log.Println("Failed to send error log: " + err.Error())
		}
	}
	if len(s.Jobs()) > 0 {
		s.Start()
	} else {
		log.Println("No cron jobs configured, exiting")
	}
	Handler := &Handler{DB: db,
		DiunWebhookEnabled: enableDiunWebhook, 
		AgentToken: agentToken, 
		DiunWebhookToken: diunWebhookToken, 
		Sessions: sessions,
		webUIPassword: webUIPassword,
	}
	http.HandleFunc("/", Handler.handleWebGui)
	http.HandleFunc("/json", Handler.handleJsonExport)
	if enableDiunWebhook {
		http.HandleFunc("/webhook", Handler.handleWebhookExport)
	}
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
    	http.ServeFile(w, r, "login.html")
	})	
	http.HandleFunc("/auth", Handler.handleLogin)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
    	rss := "OK"
		io.WriteString(w, rss)
	})	
	http.ListenAndServe(":80", nil) 
}

func podmanHealthCheck(client *http.Client, socket string, containerErrorUrl string, enableDebugging bool, cache map[string]int, hostname string, redBubble string, greenBubble string, db *sql.DB) {
	sqlInsertStatement := `
		INSERT INTO containers (ID, Name, Host, Status, ImageName, ImageDigest)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`

	sqlUpdateStatement := `
			UPDATE containers
			SET Name = $2, Host = $3, Status = $4, ImageName = $5, ImageDigest = $6
			WHERE id = $1;`
	hostname, err := os.Hostname()
	if err != nil {
		log.Println("Failed to get Hostname: " + err.Error())
	}
	// Connect to Podman socket
	connText, err := podman_binding.NewConnection(context.Background(), socket)
	if err != nil {
			log.Println(err)
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
		log.Println(err)
		os.Exit(1)
	}
	if enableDebugging {
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
		if enableDebugging {
			log.Println("Inspection read for container: " + ctrData.Name)
		}
		imageDigest := ""
		for _, img := range imageList {
			if img.ID == ctrData.Image {
				if enableDebugging {
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
		container := Container{
			ID:        	ctrData.ID,
			Name:      	ctrData.Name,
			Status:    	containerStatus,
			ImageName: 	ctrData.Config.Image,
			ImageDigest: 	imageDigest,
		}
		sqlContainerID := ""
		row := db.QueryRow("SELECT ID FROM containers WHERE ID = ?", container.ID)
		switch err := row.Scan(&sqlContainerID); err {
		case sql.ErrNoRows:
			_, err = db.Exec(sqlInsertStatement, container.ID, container.Name, hostname, container.Status, container.ImageName, container.ImageDigest, container.ImageDigest)
			if err != nil {
				log.Println(err)
			}
		case nil:
			_, err = db.Exec(sqlUpdateStatement, container.ID, container.Name, hostname, container.Status, container.ImageName, container.ImageDigest)
			if err != nil {
				log.Println(err)
			}
		default:
			log.Println(err)
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
							err := shoutrrr.Send(containerErrorUrl, "<b>ERROR:</b> Container: <b>" + ctrData.Name + "</b> on Host <b>" + hostname + "</b> has the health status: <b>" + healthstatus + "</b> " + redBubble)
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
							err := shoutrrr.Send(containerErrorUrl, "<b>RECOVERED:</b> Container: <b>" + ctrData.Name + "</b> on Host <b>" + hostname + "</b> has recovered the health status: <b>" + healthstatus + "</b> " + greenBubble)
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
							err := shoutrrr.Send(containerErrorUrl, "<b>ERROR:</b> Container: <b>" + ctrData.Name + "</b> on Host <b>" + hostname + "</b> has the container status: <b>" + containerStatus + "</b> " + redBubble)
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
							err := shoutrrr.Send(containerErrorUrl, "<b>RECOVERED:</b> Container: <b>" + ctrData.Name  + "</b> on Host <b>" + hostname + "</b> has recovered with status: <b>" + containerStatus + "</b> " + greenBubble)
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
	localContainers, err := selectAllContainers(db, hostname)
	for _, localContainer := range localContainers {
		found := false
		for _, latestContainer := range containerLatestList {
			if localContainer.ID == latestContainer.ID {
				found = true
			}
		}
		if !found {
			sqlDeleteStatement := `
			DELETE FROM containers
			WHERE ID = $1;`
			_, err = db.Exec(sqlDeleteStatement, localContainer.ID)
			if err != nil {
				log.Println("error deleting container not in remote data: ", err)
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
							err := shoutrrr.Send(containerErrorUrl, "<b>ERROR:</b> Container: <b>" + ctrData.Name + "</b> on Host <b>" + hostname + "</b> has the health status: <b>" + healthstatus + "</b> " + redBubble)
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
							err := shoutrrr.Send(containerErrorUrl, "<b>RECOVERED:</b> Container: <b>" + ctrData.Name + "</b> on Host <b>" + hostname + "</b> has recovered the health status: <b>" + healthstatus + "</b> " + greenBubble)
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
							err := shoutrrr.Send(containerErrorUrl, "<b>ERROR:</b> Container: <b>" + ctrData.Name + "</b> on Host <b>" + hostname + "</b> has the container status: <b>" + containerStatus + "</b> " + redBubble)
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
							err := shoutrrr.Send(containerErrorUrl, "<b>RECOVERED:</b> Container: <b>" + ctrData.Name  + "</b> on Host <b>" + hostname + "</b> has recovered with status: <b>" + containerStatus + "</b> " + greenBubble)
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

func getAndStoreRemoteData(remoteHostConfigs []RemoteConfig, db *sql.DB) {
	for _, config := range remoteHostConfigs {
		client := http.Client{
			Timeout: time.Second * 2, // Timeout after 2 seconds
		}
		if strings.HasPrefix(config.HostAddress, "http://") == false && strings.HasPrefix(config.HostAddress, "https://") == false {
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

				containers := []Container{}
				err = json.Unmarshal(body, &containers)
				if err != nil {
					log.Println(err)
				}

				for _, container := range containers {
					sqlInsertStatement := `
					INSERT INTO containers (ID, Name, Host, Status, ImageName, ImageDigest)
					VALUES ($1, $2, $3, $4, $5, $6)
					ON CONFLICT(ID) DO UPDATE SET
					Name = excluded.Name,
					Host = excluded.Host,
					Status = excluded.Status,
					ImageName = excluded.ImageName,
					ImageDigest = excluded.ImageDigest;`

					_, err = db.Exec(sqlInsertStatement, container.ID, container.Name, container.Host, container.Status, container.ImageName, container.ImageDigest)
					if err != nil {
						log.Println("error inserting/updating container from remote data: ", err)
					}
				}
				if len(containers) != 0 {
					localContainers, err := selectAllContainers(db, containers[0].Host)
					for _, localContainer := range localContainers {
						found := false
						for _, remoteContainer := range containers {
							if localContainer.ID == remoteContainer.ID {
								found = true
							}
						}
						if !found {
							sqlDeleteStatement := `
							DELETE FROM containers
							WHERE ID = $1;`
							_, err = db.Exec(sqlDeleteStatement, localContainer.ID)
							if err != nil {
								log.Println("error deleting container not in remote data: ", err)
							}
						}
					}
				}
			}
		}
	}
}

func (fh *Handler) handleWebGui(w http.ResponseWriter, r *http.Request) {
	if fh.webUIPassword != "" {
		c, err := r.Cookie("session_token")
		if err != nil {
			if err == http.ErrNoCookie {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		sessionToken := c.Value

		userSession, exists := fh.Sessions[sessionToken]
		if !exists {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if userSession.isExpired() {
			delete(fh.Sessions, sessionToken)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
	}
	var webContainers []ContainerWeb
	tmpl := template.Must(template.ParseFiles("layout.html"))
	containers, err := selectAllContainers(fh.DB, "")
	if err != nil {
		log.Println("error selecting containers: ", err)
	}
	for _, container := range containers {
		webContainer := ContainerWeb{
			ID: container.ID,
			Host: container.Host,
			Name: container.Name,
			Status: container.Status,
			ImageName: container.ImageName,
			ImageDigest: container.ImageDigest,
			ImageDigestNew: container.ImageDigestNew,
			EnableDiunWebhook: fh.DiunWebhookEnabled,
		}
		webContainers = append(webContainers, webContainer)
	}

	data := ContainerPageData{
		PageTitle: "ContainerMon - Monitored Containers",
		DiunWebhookEnabled: fh.DiunWebhookEnabled,
		Containers: webContainers,
	}
    tmpl.Execute(w, data)
}

func (fh *Handler) handleJsonExport(w http.ResponseWriter, r *http.Request) {
	if fh.AgentToken != "" {
		if r.Header.Get("Authorization") != fh.AgentToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}
	hostname, err := os.Hostname()
	if err != nil {
		log.Println("Failed to get Hostname: " + err.Error())
	}

	containers, err := selectAllContainers(fh.DB, hostname)
	if err != nil {
		log.Println("error selecting containers: ", err)
	}

	data, err := json.Marshal(containers)
    if err != nil {
        log.Println(err)
    }
    fmt.Fprintf(w, string(data))
}

func (fh *Handler) handleWebhookExport(w http.ResponseWriter, r *http.Request) {
	if fh.DiunWebhookToken != "" {
		if r.Header.Get("Authorization") != fh.DiunWebhookToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}
	sqlUpdateStatement := `
			UPDATE diun
			SET ImageDigest = $2
			WHERE ImageName = $1;`
	sqlInsertStatement := `
		INSERT INTO diun (ImageName, ImageDigest)
		VALUES ($1, $2)`
	duinBody := DuinWebHookBody{}
	if r.Body != nil {
		defer r.Body.Close()
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
	}

	err = json.Unmarshal(body, &duinBody)
    if err != nil {
        log.Println(err)
    }

	sqlImageName := ""
	row := fh.DB.QueryRow("SELECT ImageName FROM diun WHERE ImageName = ?", duinBody.Image)
	switch err := row.Scan(&sqlImageName); err {
	case sql.ErrNoRows:
		_ ,err = fh.DB.Exec(sqlInsertStatement, duinBody.Image, duinBody.Digest)
		if err != nil {
			log.Println(err)
		}
	case nil:
		_, err = fh.DB.Exec(sqlUpdateStatement, duinBody.Image, duinBody.Digest)
		if err != nil {
			log.Println(err)
		}
	default:
		log.Println(err)
	}
	
    fmt.Fprintf(w, "Ok")
}

func (fh *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	bodyString, errBody := io.ReadAll(r.Body)
	if errBody != nil {
		log.Println(errBody)
	}
	defer r.Body.Close()

	transmittedPassword := strings.TrimPrefix(string(bodyString), "password=")

	if fh.webUIPassword != transmittedPassword {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	sessionToken := uuid.NewString()
	expiresAt := time.Now().Add(120 * time.Minute)

	fh.Sessions[sessionToken] = session{
		expiry:   expiresAt,
	}

	http.SetCookie(w, &http.Cookie{
		Name:    "session_token",
		Value:   sessionToken,
		Expires: expiresAt,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func selectAllContainers(db *sql.DB, hostname string) ([]Container, error) {
	sqlQueryAllContainers := ""
	if hostname != "" {
		sqlQueryAllContainers = `
			SELECT containers.ID
				, containers.Name
				, containers.Host
				, containers.Status
				, containers.ImageName
				, containers.ImageDigest
				, diun.ImageDigest AS ImageDigestNew 
			FROM containers
			LEFT OUTER JOIN diun ON containers.ImageName = diun.ImageName
			WHERE Host = ?;`
	} else {
		sqlQueryAllContainers = `
			SELECT containers.ID
				, containers.Name
				, containers.Host
				, containers.Status
				, containers.ImageName
				, containers.ImageDigest
				, diun.ImageDigest AS ImageDigestNew 
			FROM containers 
			LEFT OUTER JOIN diun ON containers.ImageName = diun.ImageName
			ORDER BY Host;`
	}

	rows, err := db.Query(sqlQueryAllContainers, hostname)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var containers []Container
	for rows.Next() {
		var id, name, host, status, imageName, imageDigest string
		var imageDigestNew sql.NullString
		if err := rows.Scan(&id, &name, &host, &status, &imageName, &imageDigest, &imageDigestNew); err != nil {
			return nil, err
		}
		if !imageDigestNew.Valid {
			imageDigestNew.String = ""
		}
		container := Container{
			ID: id,
			Host: host,
			Name: name,	
			Status: status,
			ImageName: imageName,
			ImageDigest: imageDigest,
			ImageDigestNew: imageDigestNew.String,
		}
		containers = append(containers, container)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return containers, nil
}

// we'll use this method later to determine if the session has expired
func (s session) isExpired() bool {
	return s.expiry.Before(time.Now())
}