package dotnet

import (
	"github.com/jfrog/build-info-go/build/utils/dotnet"
	"github.com/jfrog/build-info-go/build/utils/dotnet/solution"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"os"
)

type NugetCommand struct {
	*DotnetCommand
}

func NewNugetCommand() *NugetCommand {
	nugetCmd := NugetCommand{&DotnetCommand{}}
	nugetCmd.SetToolchainType(dotnet.Nuget)
	return &nugetCmd
}

func (nc *NugetCommand) Run() error {
	return nc.Exec()
}

// DependencyTreeCmd loads the NuGet solution from the current working directory,
// builds the dependency tree for each project, and returns the marshalled JSON bytes.
// The caller is responsible for rendering the output.
func DependencyTreeCmd() ([]byte, error) {
	workspace, err := os.Getwd()
	if err != nil {
		return nil, errorutils.CheckError(err)
	}

	sol, err := solution.Load(workspace, "", "", log.Logger)
	if err != nil {
		return nil, err
	}

	// Create the tree for each project
	for _, project := range sol.GetProjects() {
		err = project.CreateDependencyTree(log.Logger)
		if err != nil {
			return nil, err
		}
	}
	// Build and return the tree JSON.
	content, err := sol.Marshal()
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	return content, nil
}
