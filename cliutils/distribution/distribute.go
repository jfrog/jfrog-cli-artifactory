package distribution

import (
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	distributionUtils "github.com/jfrog/jfrog-client-go/utils/distribution"
)

func CreateDefaultDistributionRules(c *components.Context) *spec.DistributionRules {
	return &spec.DistributionRules{
		DistributionRules: []spec.DistributionRule{{
			SiteName:     c.GetStringFlagValue("site"),
			CityName:     c.GetStringFlagValue("city"),
			CountryCodes: pluginsCommon.GetStringsArrFlagValue(c, "country-codes"),
		}},
	}
}

func ValidateReleaseBundleDistributeCmd(c *components.Context) error {
	if len(c.Arguments) != 2 {
		return pluginsCommon.WrongNumberOfArgumentsHandler(c)
	}
	if c.IsFlagSet("max-wait-minutes") && !c.IsFlagSet("sync") {
		return pluginsCommon.PrintHelpAndReturnError("The --max-wait-minutes option can't be used without --sync", c)
	}

	if c.IsFlagSet("dist-rules") && (c.IsFlagSet("site") || c.IsFlagSet("city") || c.IsFlagSet("country-code")) {
		return pluginsCommon.PrintHelpAndReturnError("The --dist-rules option can't be used with --site, --city or --country-code", c)
	}

	if err := ValidatePriorityFlag(c); err != nil {
		return err
	}

	return nil
}

func ValidatePriorityFlag(c *components.Context) error {
	if !c.IsFlagSet("priority") {
		return nil
	}
	priority := strings.ToLower(strings.TrimSpace(c.GetStringFlagValue("priority")))
	switch priority {
	case "low", "medium", "high":
		return nil
	default:
		return pluginsCommon.PrintHelpAndReturnError(
			fmt.Sprintf("Invalid --priority value %q. Valid values: low, medium, high", c.GetStringFlagValue("priority")), c)
	}
}

func GetPriorityFlagValue(c *components.Context) string {
	if !c.IsFlagSet("priority") {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(c.GetStringFlagValue("priority")))
}

func InitReleaseBundleDistributeCmd(c *components.Context) (distributionRules *spec.DistributionRules, maxWaitMinutes int, params distributionUtils.DistributionParams, err error) {
	if c.IsFlagSet("dist-rules") {
		distributionRules, err = spec.CreateDistributionRulesFromFile(c.GetStringFlagValue("dist-rules"))
		if err != nil {
			return
		}
	} else {
		distributionRules = CreateDefaultDistributionRules(c)
	}

	maxWaitMinutes, err = c.GetDefaultIntFlagValueIfNotSet("max-wait-minutes", 60)
	if err != nil {
		return
	}

	params = distributionUtils.NewDistributeReleaseBundleParams(c.Arguments[0], c.Arguments[1])
	return
}
