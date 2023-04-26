package git

import (
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"testing"
)

func TestClone(t *testing.T) {
	t.Run("NoReferenceSpecified", func(t *testing.T) {
		a := Activity{fs: afero.NewMemMapFs()}

		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestActivityEnvironment()
		env.RegisterActivity(a.Clone)

		params := CloneParam{
			URL: "https://github.com/temporalio/temporal.git",
		}

		_, err := env.ExecuteActivity(a.Clone, params)
		require.Error(t, err)
	})
}
