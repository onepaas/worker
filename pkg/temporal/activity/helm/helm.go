package helm

import (
	"context"
	"dagger.io/dagger"
	"embed"
	_ "embed"
	"fmt"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// chart holds our chart's template.
//
//go:embed chart/*.gohtml
//go:embed chart/templates/*.gohtml
var chart embed.FS

const (
	TemplateExtension = ".gohtml"
	// CreateChartActivityName defines Helm create chart activity name
	CreateChartActivityName = "HelmCreateChart"
	// UpgradeInstallActivityName defines Helm upgrade or install activity name
	UpgradeInstallActivityName = "HelmUpgradeInstall"
	// ErrorTypeHelmUpgradeFailed defines the Helm upgrade has failed
	ErrorTypeHelmUpgradeFailed = "ErrHelmUpgradeFailed"
)

// Activity is the representation of the Helm Temporal activities.
type Activity struct {
	templates map[string]*template.Template
}

// CreateChartParam is the CreateChart activity parameters.
type CreateChartParam struct {
	// RepositoryPath is the local path of the repository.
	RepositoryPath string
	// ApplicationName is the name of the application in a human-readable format.
	ApplicationName string
	// ApplicationSlug is the machine-readable name for the application.
	ApplicationSlug string
	// ApplicationHostname is the hostname used for the application in the Kubernetes Ingress.
	ApplicationHostname string
	// ImageRegistry is the address of the container image registry.
	ImageRegistry string
	// ImageRepository is the name of the Docker image repository used for the application.
	ImageRepository string
	// ImageTag is used to specify the version or tag of the container image used for the application in the repository.
	ImageTag string
	// KubernetesIngressClass is the name of the Kubernetes ingress class.
	KubernetesIngressClass string
}

// UpgradeInstallParam is the UpgradeInstall activity parameters.
type UpgradeInstallParam struct {
	// RepositoryPath is the local path of the repository.
	RepositoryPath string
	// ApplicationSlug is the machine-readable name for the application.
	ApplicationSlug string
	// KubernetesCA is the certificate authority used for the Kubernetes API server connection.
	KubernetesCA string
	// KubernetesAPIServer refers to the address and port number of the Kubernetes API server.
	KubernetesAPIServer string
	// KubernetesToken is a bearer token used for authentication
	KubernetesToken string
	// KubernetesNamespace specifies the namespace scope for upgrading or installing the chart.
	KubernetesNamespace string
}

type templateData struct {
	ApplicationName        string
	ApplicationSlug        string
	ApplicationHostname    string
	ImageRegistry          string
	ImageRepository        string
	ImageTag               string
	KubernetesIngressClass string
}

// New creates the Helm activity.
func New() (*Activity, error) {
	templates, err := initChartTemplates()
	if err != nil {
		return nil, err
	}

	return &Activity{
		templates: templates,
	}, nil
}

// CreateChart creates the chart.
func (a *Activity) CreateChart(ctx context.Context, param CreateChartParam) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Helm:CreateChart activity started", "Param", param)

	onepaasPath := filepath.Join(param.RepositoryPath, ".onepaas")
	chartPath := filepath.Join(onepaasPath, "chart")
	if _, err := os.Stat(chartPath); os.IsNotExist(err) {
		err = os.MkdirAll(chartPath, fs.ModeDir|fs.FileMode(0755))
		if err != nil {
			logger.Error("Helm:CreateChart unable to create the directory", "Error", err, "Path", chartPath)

			return err
		}
	} else {
		return nil
	}

	chartTemplatesPath := filepath.Join(chartPath, "templates")
	if _, err := os.Stat(chartTemplatesPath); os.IsNotExist(err) {
		err = os.MkdirAll(chartTemplatesPath, fs.ModeDir|fs.FileMode(0755))
		if err != nil {
			logger.Error("Helm:CreateChart unable to create the directory", "Error", err, "Path", chartTemplatesPath)

			return err
		}
	}

	for path, tpl := range a.templates {
		target := filepath.Join(onepaasPath, filepath.Dir(path), strings.TrimSuffix(tpl.Name(), TemplateExtension))

		wr, err := os.Create(target)
		if err != nil {
			logger.Error("Helm:CreateChart unable to create the file", "Error", err, "Path", target)

			return err
		}

		data := templateData{
			ApplicationName:        param.ApplicationName,
			ApplicationSlug:        param.ApplicationSlug,
			ApplicationHostname:    param.ApplicationHostname,
			ImageRegistry:          param.ImageRegistry,
			ImageRepository:        param.ImageRepository,
			ImageTag:               param.ImageTag,
			KubernetesIngressClass: param.KubernetesIngressClass,
		}

		err = tpl.Execute(wr, data)
		if err != nil {
			logger.Error("Helm:CreateChart unable to execute the template", "Error", err, "Template", tpl.Name())

			return err
		}
	}

	return nil
}

// UpgradeInstall upgrades the existing release or installs a new one if it doesn't exist.
func (a *Activity) UpgradeInstall(ctx context.Context, param UpgradeInstallParam) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Helm:UpgradeInstall activity started", "Param", param)

	client, err := dagger.Connect(context.Background(), dagger.WithWorkdir(param.RepositoryPath))
	if err != nil {
		logger.Error("Helm:UpgradeInstall unable to connect to the dagger service", "Error", err)

		return err
	}

	defer func(client *dagger.Client) {
		err := client.Close()
		if err != nil {
			logger.Error("Helm:UpgradeInstall unable to close the dagger client", "Error", err)
		}
	}(client)

	// get reference to the chart directory
	chartPath := client.Host().Directory("./.onepaas/chart")
	helmImg := client.Container().
		From("alpine/helm:3.11.3").
		WithMountedDirectory("/chart", chartPath).WithWorkdir("/chart").
		WithNewFile("/chart/ca.crt", dagger.ContainerWithNewFileOpts{Contents: param.KubernetesCA}).
		WithEnvVariable("HELM_KUBEAPISERVER", param.KubernetesAPIServer).
		WithEnvVariable("HELM_KUBECAFILE", "/chart/ca.crt").
		WithSecretVariable("HELM_KUBETOKEN", client.SetSecret("kubeToken", param.KubernetesToken)).
		WithExec([]string{"repo", "add", "companyinfo", "https://companyinfo.github.io/helm-charts"}).
		WithExec([]string{"dependency", "build"}).
		WithExec([]string{
			"--namespace", param.KubernetesNamespace,
			"upgrade",
			"--install",
			"--cleanup-on-fail",
			param.ApplicationSlug,
			".",
		})

	exitCode, err := helmImg.ExitCode(ctx)
	if err != nil {
		logger.Error("Helm:UpgradeInstall no command has been executed", "Error", err)

		return err
	}

	if exitCode != 0 {
		return temporal.NewNonRetryableApplicationError(
			"the Helm command returned non-zero exit code",
			ErrorTypeHelmUpgradeFailed,
			nil,
			"ExitCode", exitCode,
		)
	}

	return nil
}

func initChartTemplates() (map[string]*template.Template, error) {
	templates := make(map[string]*template.Template)

	err := fs.WalkDir(chart, ".", func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error walking chart's assets: %w", err)
		}

		if !info.IsDir() && strings.HasSuffix(path, TemplateExtension) {
			t, err := template.ParseFS(chart, path)
			if err != nil {
				return fmt.Errorf("error parsing template for the file %s: %w", path, err)
			}

			templates[path] = t
		}

		return nil
	})

	return templates, err
}
