package Homepagemodels

type HomepageImageStatus struct {
	ContainerImageStatus  []ContainerImageStatus
}

type ContainerImageStatus struct {
	Container       string
	ImageStatus   	string
}

type HomepageContainerStatus struct {
	ContainerStatus  []ContainerStatus
}

type ContainerStatus struct {
	Container       string
	ContainerStatus   	string
}