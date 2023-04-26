package git

import (
	"context"
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	desfacer "gopkg.in/jfontan/go-billy-desfacer.v0"
)

const (
	// ProjectsBaseDir defines projects base directory
	ProjectsBaseDir = "/tmp/projects"

	// CloneActivityName defines Git clone activity name
	CloneActivityName = "GitClone"

	// ErrorTypeNoReferenceSpecified defines the no reference specified error
	ErrorTypeNoReferenceSpecified = "ErrNoReferenceSpecified"
)

// Activity is the representation of a Git Temporal activity.
type Activity struct {
	fs afero.Fs
}

// CloneParam is the Clone activity parameters.
type CloneParam struct {
	// URL is the repository URL
	URL string
	// Branch is the branch name to check out
	Branch string
	// Tag is the tag name to check out
	Tag string
	// Ref is the ref to check out e.g. refs/changes/04/691202/5
	Ref string
}

type CloneResult struct {
	RepositoryPath string
}

// New creates the Git activity.
func New(filesystem afero.Fs) *Activity {
	return &Activity{
		fs: filesystem,
	}
}

// Clone clones the local file into the local storage.
func (a *Activity) Clone(ctx context.Context, param CloneParam) (CloneResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Git:Clone activity started", "Param", param)

	projectDirName, err := uuid.NewRandom()
	if err != nil {
		logger.Error("Git:Clone unable to generate an uuid", "Error", err)

		return CloneResult{}, temporal.NewNonRetryableApplicationError(
			"unable to generate an uuid", "", err,
		)
	}

	// Worktree filesystem
	worktreeFS, _ := desfacer.New(afero.NewBasePathFs(a.fs, ProjectsBaseDir)).
		Chroot(fmt.Sprintf("/%s", projectDirName))

	// Git objects (.git dir) filesystem
	gitFS, _ := desfacer.New(afero.NewBasePathFs(a.fs, ProjectsBaseDir)).
		Chroot(fmt.Sprintf("/%s/.git", projectDirName))

	// Git objects (.git dir) storage
	storer := filesystem.NewStorage(gitFS, cache.NewObjectLRUDefault())

	var refName plumbing.ReferenceName
	switch {
	case param.Branch != "":
		refName = plumbing.NewBranchReferenceName(param.Branch)

	case param.Tag != "":
		refName = plumbing.NewTagReferenceName(param.Tag)

	case param.Ref != "":
		refName = plumbing.ReferenceName(param.Ref)

	default:
		return CloneResult{}, temporal.NewNonRetryableApplicationError(
			"no reference specified", ErrorTypeNoReferenceSpecified, nil,
		)
	}

	_, err = git.CloneContext(context.Background(), storer, worktreeFS, &git.CloneOptions{
		URL:           param.URL,
		Depth:         1,
		Progress:      nil,
		ReferenceName: refName,
		SingleBranch:  true,
	})

	if err != nil {
		logger.Error("Git:Clone unable to clone the repository", "Error", err)

		return CloneResult{}, err
	}

	logger.Info("Git:Clone activity was executed successfully.")

	return CloneResult{RepositoryPath: fmt.Sprintf("%s/%s", ProjectsBaseDir, projectDirName)}, nil
}
