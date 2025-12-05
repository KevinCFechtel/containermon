package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/containrrr/shoutrrr"

	"github.com/go-co-op/gocron/v2"

	Dockerhandler "github.com/KevinCFechtel/containermon/handler/container/docker"
	Podmanhandler "github.com/KevinCFechtel/containermon/handler/container/podman"
	Databasehandler "github.com/KevinCFechtel/containermon/handler/database"
	Remotehandler "github.com/KevinCFechtel/containermon/handler/remotes"
	Webhandler "github.com/KevinCFechtel/containermon/handler/web"
	Remotemodels "github.com/KevinCFechtel/containermon/models/remotes"
)

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
	var webSessionExpirationTime int
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
	remoteHostConfigs := []Remotemodels.RemoteConfig{}
	for _, e := range os.Environ() {
		if i := strings.Index(e, "="); i >= 0 {
			if strings.HasPrefix(e, "REMOTE_CONFIG_HOST_") {
				configCoutnter := strings.TrimPrefix(e[:i], "REMOTE_CONFIG_HOST_")
				remoteToken := os.Getenv("REMOTE_CONFIG_TOKEN_" + configCoutnter)
				remoteHostConfigs = append(remoteHostConfigs, Remotemodels.RemoteConfig{ 
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
	flag.IntVar(&webSessionExpirationTime, "webSessionExpirationTime", -1, "Web UI session expiration time in minutes")
	if(webSessionExpirationTime == -1) {
		webSessionExpirationTimeEnvar, err := strconv.Atoi(os.Getenv("WEB_SESSION_EXPIRATION_TIME"))
		if err != nil {
			webSessionExpirationTime = 120
		} else {
			webSessionExpirationTime = webSessionExpirationTimeEnvar
		}

	}

	socket := ""
	if strings.Contains(socketPath,"podman") {
		socket = "unix:" + socketPath
	} else {
		socket = "unix://" + socketPath
	}

 hostname, err := os.Hostname()
 if err != nil {
  log.Println("Failed to get Hostname: " + err.Error())
 }
	DBHandler := Databasehandler.NewHandler(dbPath)
	Podmanhandler := Podmanhandler.NewHandler(DBHandler, socket, containerErrorUrl, enableDebugging, hostname, redBubble, greenBubble)
	Dockerhandler := Dockerhandler.NewHandler(DBHandler, socket, containerErrorUrl, enableDebugging, hostname, redBubble, greenBubble)
	Webhandler := Webhandler.NewWebHandler(DBHandler, enableDiunWebhook, agentToken, diunWebhookToken, webUIPassword, webSessionExpirationTime)
	Remotehandler := Remotehandler.NewHandler(DBHandler)

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

					if strings.Contains(socketPath,"podman") {
						if enableDebugging {
							log.Println("Using Podman health check")
						}
						Podmanhandler.PodmanHealthCheck()
					} else {
						if enableDebugging {
							log.Println("Using Docker health check")
						}
						Dockerhandler.DockerHealthCheck()
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
					Remotehandler.GetAndStoreRemoteData(remoteHostConfigs)
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


	http.HandleFunc("/", Webhandler.HandleWebGui)
	http.HandleFunc("/json", Webhandler.HandleJsonExport)
	if enableDiunWebhook {
		http.HandleFunc("/webhook", Webhandler.HandleWebhookExport)
		http.HandleFunc("/manualUpdate", Webhandler.HandleManualUpdate)
	}
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
    	http.ServeFile(w, r, "login.html")
	})	
	http.HandleFunc("/auth", Webhandler.HandleLogin)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
    	rss := "OK"
		io.WriteString(w, rss)
	})	
	http.ListenAndServe(":80", nil) 
}