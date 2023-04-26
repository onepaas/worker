package docker

import (
	"context"
	"dagger.io/dagger"
	"fmt"
	"go.temporal.io/sdk/activity"
)

// BuildAndPublishActivityName defines Docker build and publish activity name
const BuildAndPublishActivityName = "DockerBuildAndPublish"
const DefaultDockerfilePath = "./Dockerfile"
const DefaultRegistryAddress = "docker.io"
const DefaultImageTag = "latest"

// Activity is the representation of the Docker Temporal activities.
type Activity struct{}

// BuildAndPublishParam is the BuildAndPublish activity parameters.
type BuildAndPublishParam struct {
	// WorkDirectory specifies the working directory to use in the container.
	WorkDirectory string
	// Dockerfile specifies the path to the Dockerfile that should be used. By default, it is set to './Dockerfile'.
	Dockerfile string
	// RegistryAddress is the address of the container image registry.
	RegistryAddress string
	// RegistryUsername is the username used for authentication to access a container image registry.
	RegistryUsername string
	// RegistrySecret is the secret used for authentication to access a container image registry.
	RegistrySecret string
	// ImageRepository is a storage location that contains all the versions of a particular image.
	ImageRepository string
	// ImageTag refers to the specific version or tag of an image stored in a repository.
	ImageTag string
}

func New() Activity {
	return Activity{}
}

// BuildAndPublish builds a container image and publishes it to a container registry.
func (a *Activity) BuildAndPublish(ctx context.Context, param BuildAndPublishParam) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Docker:BuildAndPublish activity started", "Param", param)

	client, err := dagger.Connect(ctx, dagger.WithWorkdir(param.WorkDirectory))
	if err != nil {
		logger.Error("Docker:BuildAndPublish unable to connect to the dagger service", "Error", err)

		return err
	}

	defer func(client *dagger.Client) {
		err := client.Close()
		if err != nil {
			logger.Error("Docker:BuildAndPublish unable to close the dagger client", "Error", err)
		}
	}(client)

	if param.Dockerfile == "" {
		param.Dockerfile = DefaultDockerfilePath
	}

	if param.RegistryAddress == "" {
		param.RegistryAddress = DefaultRegistryAddress
	}

	if param.ImageTag == "" {
		param.ImageTag = DefaultImageTag
	}

	publishAddress := fmt.Sprintf("%s/%s:%s", param.RegistryAddress, param.ImageRepository, param.ImageTag)

	_, err = client.Host().Directory(".").
		DockerBuild(dagger.DirectoryDockerBuildOpts{Dockerfile: param.Dockerfile}).
		WithRegistryAuth(
			param.RegistryAddress,
			param.RegistryUsername,
			client.SetSecret("registrySecret", param.RegistrySecret),
		).
		Publish(ctx, publishAddress)
	if err != nil {
		logger.Error("Docker:BuildAndPublish unable to build and publish the container image", "Error", err)

		return err
	}

	logger.Info("Docker:BuildAndPublish activity was executed successfully.")

	return nil
}
