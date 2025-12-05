package DatabaseHandler

import (
	"database/sql"

	Containermodels "github.com/KevinCFechtel/containermon/models/container"
)

type DatabaseHandler struct {
    DB *sql.DB
}

func selectAllContainers(db *sql.DB, hostname string) ([]Containermodels.Container, error) {
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

