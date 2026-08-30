package commands

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/stretchr/testify/assert"
)

func TestReleaseBundleDistributeCommandSetPriority(t *testing.T) {
	cmd := NewReleaseBundleDistributeCommand().
		SetPriority("high").
		SetDistributionRules(&spec.DistributionRules{
			DistributionRules: []spec.DistributionRule{{SiteName: "edge1"}},
		})
	assert.Equal(t, "high", cmd.priority)
}

func TestReleaseBundleRemoteDeleteCommandSetPriority(t *testing.T) {
	cmd := NewReleaseBundleRemoteDeleteCommand().
		SetPriority("low").
		SetDistributionRules(&spec.DistributionRules{
			DistributionRules: []spec.DistributionRule{{SiteName: "edge1"}},
		})
	assert.Equal(t, "low", cmd.priority)
}
