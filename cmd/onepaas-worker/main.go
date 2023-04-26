package main

import (
	"github.com/onepaas/worker/pkg/temporal/activity/docker"
	"github.com/onepaas/worker/pkg/temporal/activity/git"
	"github.com/onepaas/worker/pkg/temporal/activity/helm"
	cdworker "github.com/onepaas/worker/pkg/temporal/worker"
	"github.com/onepaas/worker/pkg/temporal/workflow/deployment"
	"github.com/rs/zerolog/log"
	"github.com/spf13/afero"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"os"
)

func main() {
	clientOptions := client.Options{
		HostPort:  os.Getenv("TEMPORAL_ADDRESS"),
		Namespace: "default",
	}

	c, err := client.Dial(clientOptions)
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("Unable to dial Temporal server")
	}
	defer c.Close()

	deploymentWorkflow := deployment.New()

	w := worker.New(c, cdworker.OnePaaSTaskQueue, worker.Options{})
	w.RegisterWorkflowWithOptions(deploymentWorkflow.Deploy, workflow.RegisterOptions{Name: deployment.WorkflowName})

	gitActivity := git.New(afero.NewOsFs())
	w.RegisterActivityWithOptions(gitActivity.Clone, activity.RegisterOptions{Name: git.CloneActivityName})

	dockerActivity := docker.New()
	w.RegisterActivityWithOptions(dockerActivity.BuildAndPublish, activity.RegisterOptions{Name: docker.BuildAndPublishActivityName})

	helmActivity, err := helm.New()
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("Unable to initialize Helm activity")
	}
	w.RegisterActivityWithOptions(helmActivity.CreateChart, activity.RegisterOptions{Name: helm.CreateChartActivityName})
	w.RegisterActivityWithOptions(helmActivity.UpgradeInstall, activity.RegisterOptions{Name: helm.UpgradeInstallActivityName})

	err = w.Run(worker.InterruptCh())
	if err != nil {
		log.Fatal().
			Err(err).
			Msg("Unable to start worker")
	}
}
