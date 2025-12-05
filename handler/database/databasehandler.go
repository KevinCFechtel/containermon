package DatabaseHandler

import (
	"database/sql"
	"log"

	Containermodels "github.com/KevinCFechtel/containermon/models/container"
)

type Handler struct {
    db *sql.DB
}

func NewHandler(dbPath string) *Handler {
	newDB, _ := sql.Open("sqlite", "file:"+dbPath)
	sqlCreateTable := `
	CREATE TABLE IF NOT EXISTS containers (
		ID TEXT PRIMARY KEY,
		Host TEST,
		Name TEXT,
		Status TEXT,
		ImageName TEXT,
		ImageDigest TEXT
	);`
	_, err := newDB.Exec(sqlCreateTable)
	if err != nil {
		log.Println("error creating containers table: ", err)
	}
	//defer newDB.Close()

	sqlCreateTable = `
	CREATE TABLE IF NOT EXISTS diun (
		ImageName TEXT PRIMARY KEY,
		ImageDigest TEXT
	);`
	_, err = newDB.Exec(sqlCreateTable)
	if err != nil {
		log.Println("error creating containers table: ", err)
	}
	return &Handler{
		db: newDB,
	}
}

func (h *Handler) SelectAllContainers(hostname string) ([]Containermodels.Container, error) {
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

	rows, err := h.db.Query(sqlQueryAllContainers, hostname)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var containers []Containermodels.Container
	for rows.Next() {
		var id, name, host, status, imageName, imageDigest string
		var imageDigestNew sql.NullString
		if err := rows.Scan(&id, &name, &host, &status, &imageName, &imageDigest, &imageDigestNew); err != nil {
			return nil, err
		}
		if !imageDigestNew.Valid {
			imageDigestNew.String = ""
		}
		container := Containermodels.Container{
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

func (h *Handler) InsertOrUpdateContainer(container Containermodels.Container, hostname string) {
	sqlInsertStatement := `
		INSERT INTO containers (ID, Name, Host, Status, ImageName, ImageDigest)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`

	sqlUpdateStatement := `
			UPDATE containers
			SET Name = $2, Host = $3, Status = $4, ImageName = $5, ImageDigest = $6
			WHERE id = $1;`
			
	sqlContainerID := ""	
	row := h.db.QueryRow("SELECT ID FROM containers WHERE ID = ?", container.ID)
	switch err := row.Scan(&sqlContainerID); err {
		case sql.ErrNoRows:
			_, err = h.db.Exec(sqlInsertStatement, container.ID, container.Name, hostname, container.Status, container.ImageName, container.ImageDigest, container.ImageDigest)
			if err != nil {
				log.Println(err)
			}
		case nil:
			_, err = h.db.Exec(sqlUpdateStatement, container.ID, container.Name, hostname, container.Status, container.ImageName, container.ImageDigest)
			if err != nil {
				log.Println(err)
			}
		default:
			log.Println(err)
	}
}	

func (h *Handler) DeleteContainer(containerID string) {
	sqlDeleteStatement := `
			DELETE FROM containers
			WHERE ID = $1;`
	_, err := h.db.Exec(sqlDeleteStatement, containerID)
	if err != nil {
		log.Println("error deleting container not in remote data: ", err)
	}		
}

func (h *Handler) InsortOrUpdateImageDigest(imageName string, imageDigest string) {
	sqlUpdateStatement := `
			UPDATE diun
			SET ImageDigest = $2
			WHERE ImageName = $1;`
	sqlInsertStatement := `
		INSERT INTO diun (ImageName, ImageDigest)
		VALUES ($1, $2)`
	
	sqlImageName := ""
	row := h.db.QueryRow("SELECT ImageName FROM diun WHERE ImageName = ?", imageName)
	switch err := row.Scan(&sqlImageName); err {
	case sql.ErrNoRows:
		_ ,err = h.db.Exec(sqlInsertStatement, imageName, imageDigest)
		if err != nil {
			log.Println(err)
		}
	case nil:
		_, err = h.db.Exec(sqlUpdateStatement, imageName, imageDigest)
		if err != nil {
			log.Println(err)
		}
	default:
		log.Println(err)
	}
}