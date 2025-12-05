package Webmodels

import "time"

type Session struct {
	Expiry   time.Time
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

type ContainerPageData struct {
    PageTitle 	string
	DiunWebhookEnabled bool
    Containers 	[]ContainerWeb
}

// we'll use this method later to determine if the session has expired
func (s Session) IsExpired() bool {
	return s.Expiry.Before(time.Now())
}