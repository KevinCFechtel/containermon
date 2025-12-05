package Webhandler

import (
	"database/sql"
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

	Webmodels "github.com/KevinCFechtel/containermon/models/web"
)

type WebHandler struct {
    DB *sql.DB
	DiunWebhookEnabled bool
	AgentToken string
	DiunWebhookToken string
	Sessions map[string]Webmodels.Session
	webUIPassword string
	webSessionExpirationTime int
}

func (fh *WebHandler) handleWebGui(w http.ResponseWriter, r *http.Request) {
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
	var webContainers []Webmodels.ContainerWeb
	tmpl := template.Must(template.ParseFiles("layout.html"))
	containers, err := selectAllContainers(fh.DB, "")
	if err != nil {
		log.Println("error selecting containers: ", err)
	}
	for _, container := range containers {
		webContainer := Webmodels.ContainerWeb{
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

	data := Webmodels.ContainerPageData{
		PageTitle: "ContainerMon - Monitored Containers",
		DiunWebhookEnabled: fh.DiunWebhookEnabled,
		Containers: webContainers,
	}
    tmpl.Execute(w, data)
}

func (fh *WebHandler) handleJsonExport(w http.ResponseWriter, r *http.Request) {
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

func (fh *WebHandler) handleWebhookExport(w http.ResponseWriter, r *http.Request) {
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
	duinBody := Webmodels.DuinWebHookBody{}
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

func (fh *WebHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	transmittedPassword := r.FormValue("password")

	if fh.webUIPassword != transmittedPassword {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	sessionToken := uuid.NewString()
	expiresAt := time.Now().Add(time.Duration(fh.webSessionExpirationTime) * time.Minute)

	fh.Sessions[sessionToken] = Webmodels.Session{
		Expiry:   expiresAt,
	}

	http.SetCookie(w, &http.Cookie{
		Name:    "session_token",
		Value:   sessionToken,
		Expires: expiresAt,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (fh *WebHandler) handleManualUpdate(w http.ResponseWriter, r *http.Request) {
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
	sqlUpdateStatement := `
			UPDATE diun
			SET ImageDigest = $2
			WHERE ImageName = $1;`
	sqlInsertStatement := `
		INSERT INTO diun (ImageName, ImageDigest)
		VALUES ($1, $2)`
	duinBody := Webmodels.DuinWebHookBody{}

	duinBody.Image = r.FormValue("imageName")
	duinBody.Digest = r.FormValue("imageDigest")

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
	
    http.Redirect(w, r, "/", http.StatusSeeOther)
}