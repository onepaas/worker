package deployment

import (
	"github.com/onepaas/worker/pkg/temporal/activity/docker"
	"github.com/onepaas/worker/pkg/temporal/activity/git"
	"github.com/onepaas/worker/pkg/temporal/activity/helm"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
	"time"
)

// Workflow is the representation of a deployment Temporal workflow.
type Workflow struct{}

// DeployParam is the Deploy workflow parameters.
type DeployParam struct {
	RepositoryURL          string
	RepositoryBranch       string
	RepositoryTag          string
	RepositoryRef          string
	RegistryAddress        string
	RegistryUsername       string
	RegistrySecret         string
	ImageRepository        string
	ImageTag               string
	ApplicationName        string
	ApplicationSlug        string
	ApplicationHostname    string
	KubernetesIngressClass string
	KubernetesCA           string
	KubernetesToken        string
	KubernetesAPIServer    string
	KubernetesNamespace    string
}

// WorkflowName represents the OrderDocument workflow name
const WorkflowName = "DeployApplication"

// New creates the deployment workflow.
func New() Workflow {
	return Workflow{}
}

// Deploy deploys the application
func (w *Workflow) Deploy(ctx workflow.Context, param DeployParam) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("Deploy workflow started", "Param", param)

	activityOptions := workflow.ActivityOptions{
		ScheduleToCloseTimeout: 60 * time.Minute,
		StartToCloseTimeout:    10 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 1,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	var gitRepository *git.CloneResult
	gitCloneParam := git.CloneParam{
		URL:    param.RepositoryURL,
		Branch: param.RepositoryBranch,
		Tag:    param.RepositoryTag,
		Ref:    param.RepositoryRef,
	}
	f := workflow.ExecuteActivity(ctx, git.CloneActivityName, gitCloneParam)
	if err := f.Get(ctx, &gitRepository); err != nil {
		logger.Error("OnePaaS:Deploy unable to execute Git:Clone activity", "Error", err)

		return err
	}

	f = workflow.ExecuteActivity(
		ctx,
		docker.BuildAndPublishActivityName,
		docker.BuildAndPublishParam{
			WorkDirectory:    gitRepository.RepositoryPath,
			RegistryAddress:  param.RegistryAddress,
			RegistryUsername: param.RegistryUsername,
			RegistrySecret:   param.RegistrySecret,
			ImageRepository:  param.ImageRepository,
			ImageTag:         param.ImageTag,
		},
	)
	if err := f.Get(ctx, nil); err != nil {
		logger.Error("OnePaaS:Deploy unable to execute Docker:BuildAndPublish activity", "Error", err)

		return err
	}

	createChartParam := helm.CreateChartParam{
		RepositoryPath:         gitRepository.RepositoryPath,
		ApplicationName:        param.ApplicationName,
		ApplicationSlug:        param.ApplicationSlug,
		ApplicationHostname:    param.ApplicationHostname,
		ImageRegistry:          param.RegistryAddress,
		ImageRepository:        param.ImageRepository,
		ImageTag:               param.ImageTag,
		KubernetesIngressClass: param.KubernetesIngressClass,
	}
	f = workflow.ExecuteActivity(ctx, helm.CreateChartActivityName, createChartParam)
	if err := f.Get(ctx, nil); err != nil {
		logger.Error("OnePaaS:Deploy unable to execute Helm:CreateChart activity", "Error", err)

		return err
	}

	upgradeInstallParam := helm.UpgradeInstallParam{
		RepositoryPath:      gitRepository.RepositoryPath,
		ApplicationSlug:     param.ApplicationSlug,
		KubernetesCA:        param.KubernetesCA,
		KubernetesToken:     param.KubernetesToken,
		KubernetesAPIServer: param.KubernetesAPIServer,
		KubernetesNamespace: param.KubernetesNamespace,
	}
	f = workflow.ExecuteActivity(ctx, helm.UpgradeInstallActivityName, upgradeInstallParam)
	if err := f.Get(ctx, nil); err != nil {
		logger.Error("OnePaaS:Deploy unable to execute Helm:UpgradeInstall activity", "Error", err)

		return err
	}

	return nil
}
