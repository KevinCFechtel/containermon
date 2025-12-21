package Webhandler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	Databasehandler "github.com/KevinCFechtel/containermon/handler/database"
	Webmodels "github.com/KevinCFechtel/containermon/models/web"
)

type WebHandler struct {
    DBHandler *Databasehandler.Handler
	diunWebhookEnabled bool
	agentToken string
	diunWebhookToken string
	sessions map[string]Webmodels.Session
	webUIPassword string
	webSessionExpirationTime int
}

func NewWebHandler(newDBHandler *Databasehandler.Handler, diunWebhookEnabled bool, agentToken string, diunWebhookToken string, webUIPassword string, webSessionExpirationTime int) *WebHandler {
	return &WebHandler{
		DBHandler: newDBHandler,
		diunWebhookEnabled: diunWebhookEnabled,
		agentToken: agentToken,
		diunWebhookToken: diunWebhookToken,
		sessions: make(map[string]Webmodels.Session),
		webUIPassword: webUIPassword,
		webSessionExpirationTime: webSessionExpirationTime,
	}
}

func (h *WebHandler) HandleWebGui(w http.ResponseWriter, r *http.Request) {
	if h.webUIPassword != "" {
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

		userSession, exists := h.sessions[sessionToken]
		if !exists {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if userSession.IsExpired() {
			delete(h.sessions, sessionToken)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
	}
	var webContainers []Webmodels.ContainerWeb
	tmpl := template.Must(template.ParseFiles("layout.html"))
	containers, err := h.DBHandler.SelectAllContainers("")
	if err != nil {
		log.Println("error selecting containers: ", err)
	} else {
		for _, container := range containers {
			webContainer := Webmodels.ContainerWeb{
				ID: container.ID,
				Host: container.Host,
				Name: container.Name,
				Status: container.Status,
				ImageName: container.ImageName,
				ImageDigest: container.ImageDigest,
				ImageDigestNew: container.ImageDigestNew,
				EnableDiunWebhook: h.diunWebhookEnabled,
			}
			webContainers = append(webContainers, webContainer)
		}
	}

	data := Webmodels.ContainerPageData{
		PageTitle: "Monitored Containers",
		DiunWebhookEnabled: h.diunWebhookEnabled,
		Containers: webContainers,
	}
    tmpl.Execute(w, data)
}

func (h *WebHandler) HandleJsonExport(w http.ResponseWriter, r *http.Request) {
	if h.agentToken != "" {
		if r.Header.Get("Authorization") != h.agentToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}
	hostname, err := os.Hostname()
	if err != nil {
		log.Println("Failed to get Hostname: " + err.Error())
	}

	var data []byte
	containers, err := h.DBHandler.SelectAllContainers(hostname)
	if err != nil {
		log.Println("error selecting containers: ", err)
	} else {
		data, err = json.Marshal(containers)
		if err != nil {
			log.Println(err)
		}
	}
    fmt.Fprintf(w, "%s", string(data))
}

func (h *WebHandler) HandleWebhookExport(w http.ResponseWriter, r *http.Request) {
	if h.diunWebhookToken != "" {
		if r.Header.Get("Authorization") != h.diunWebhookToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}
	
	diunBody := Webmodels.DuinWebHookBody{}
	if r.Body != nil {
		defer r.Body.Close()
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
	}

	err = json.Unmarshal(body, &diunBody)
    if err != nil {
        log.Println(err)
    }

	h.DBHandler.InsortOrUpdateImageDigest(diunBody.Image, diunBody.Digest)
	
    fmt.Fprintf(w, "Ok")
}

func (fh *WebHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	transmittedPassword := r.FormValue("password")

	if fh.webUIPassword != transmittedPassword {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	sessionToken := uuid.NewString()
	expiresAt := time.Now().Add(time.Duration(fh.webSessionExpirationTime) * time.Minute)

	fh.sessions[sessionToken] = Webmodels.Session{
		Expiry:   expiresAt,
	}

	http.SetCookie(w, &http.Cookie{
		Name:    "session_token",
		Value:   sessionToken,
		Expires: expiresAt,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *WebHandler) HandleManualUpdate(w http.ResponseWriter, r *http.Request) {
	if h.webUIPassword != "" {
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

		userSession, exists := h.sessions[sessionToken]
		if !exists {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if userSession.IsExpired() {
			delete(h.sessions, sessionToken)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
	}
	diunBody := Webmodels.DuinWebHookBody{}

	diunBody.Image = r.FormValue("imageName")
	diunBody.Digest = r.FormValue("imageDigest")

	h.DBHandler.InsortOrUpdateImageDigest(diunBody.Image, diunBody.Digest)
	
    http.Redirect(w, r, "/", http.StatusSeeOther)
}