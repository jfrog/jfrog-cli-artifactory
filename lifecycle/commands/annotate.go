package commands

import (
	"fmt"
	"github.com/jfrog/gofrog/log"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/lifecycle"
	"github.com/jfrog/jfrog-client-go/lifecycle/services"
)

const minSetTagArtifactoryVersion = "7.111.0"

type ReleaseBundleAnnotateCommand struct {
	releaseBundleCmd
	tag                       string
	tagExist                  bool
	props                     string
	propsExist                bool
	deleteProps               string
	deletePropsExist          bool
	recursive                 bool
	validateVersionFunc       func(*config.ServerDetails, string) error
	getPrerequisitesFunc      func() (*lifecycle.LifecycleServicesManager, services.ReleaseBundleDetails, services.CommonOptionalQueryParams, error)
	annotateReleaseBundleFunc func(*ReleaseBundleAnnotateCommand, *lifecycle.LifecycleServicesManager,
		services.ReleaseBundleDetails, services.CommonOptionalQueryParams) error
}

func NewReleaseBundleAnnotateCommand() *ReleaseBundleAnnotateCommand {
	cmd := &ReleaseBundleAnnotateCommand{}
	cmd.annotateReleaseBundleFunc = DefaultAnnotateReleaseBundle
	cmd.validateVersionFunc = ValidateFeatureSupportedVersion
	cmd.getPrerequisitesFunc = func() (*lifecycle.LifecycleServicesManager, services.ReleaseBundleDetails, services.CommonOptionalQueryParams, error) {
		return DefaultGetPrerequisites(cmd)
	}
	return cmd
}

func DefaultGetPrerequisites(rba *ReleaseBundleAnnotateCommand) (*lifecycle.LifecycleServicesManager, services.ReleaseBundleDetails, services.CommonOptionalQueryParams, error) {
	return rba.getPrerequisites()
}

func (rba *ReleaseBundleAnnotateCommand) SetRecursive(recursive, flagIsSet bool) *ReleaseBundleAnnotateCommand {
	if flagIsSet {
		rba.recursive = recursive
	} else {
		rba.recursive = true
	}

	return rba
}

func (rba *ReleaseBundleAnnotateCommand) SetServerDetails(serverDetails *config.ServerDetails) *ReleaseBundleAnnotateCommand {
	rba.serverDetails = serverDetails
	return rba
}

func (rba *ReleaseBundleAnnotateCommand) SetReleaseBundleName(releaseBundleName string) *ReleaseBundleAnnotateCommand {
	rba.releaseBundleName = releaseBundleName
	return rba
}

func (rba *ReleaseBundleAnnotateCommand) SetReleaseBundleVersion(releaseBundleVersion string) *ReleaseBundleAnnotateCommand {
	rba.releaseBundleVersion = releaseBundleVersion
	return rba
}

func (rba *ReleaseBundleAnnotateCommand) SetReleaseBundleProject(rbProjectKey string) *ReleaseBundleAnnotateCommand {
	rba.rbProjectKey = rbProjectKey
	return rba
}

func (rba *ReleaseBundleAnnotateCommand) SetTag(tag string, exist bool) *ReleaseBundleAnnotateCommand {
	rba.tag = tag
	rba.tagExist = exist
	return rba
}

func (rba *ReleaseBundleAnnotateCommand) SetProps(props string) *ReleaseBundleAnnotateCommand {
	rba.props = props
	rba.propsExist = props != ""
	return rba
}

func (rba *ReleaseBundleAnnotateCommand) DeleteProps(deleteProps string) *ReleaseBundleAnnotateCommand {
	rba.deleteProps = deleteProps
	rba.deletePropsExist = deleteProps != ""
	return rba
}

func (rba *ReleaseBundleAnnotateCommand) ServerDetails() (*config.ServerDetails, error) {
	return rba.serverDetails, nil
}

func (rba *ReleaseBundleAnnotateCommand) CommandName() string {
	return "rb_annotate"
}

func (rba *ReleaseBundleAnnotateCommand) Run() error {
	if err := rba.validateVersionFunc(rba.serverDetails, minSetTagArtifactoryVersion); err != nil {
		return err
	}

	servicesManager, rbDetails, queryParams, err := rba.getPrerequisitesFunc()
	if err != nil {
		return err
	}

	err = rba.annotateReleaseBundleFunc(rba, servicesManager, rbDetails, queryParams)
	if err != nil {
		return err
	}
	log.Info(fmt.Sprintf("Successfully annotated release bundle: %s/%s", rbDetails.ReleaseBundleName, rbDetails.ReleaseBundleVersion))

	return nil
}

func DefaultAnnotateReleaseBundle(rba *ReleaseBundleAnnotateCommand, manager *lifecycle.LifecycleServicesManager,
	details services.ReleaseBundleDetails, params services.CommonOptionalQueryParams) error {
	return rba.annotateReleaseBundle(manager, details, params, rba)
}

func (rba *ReleaseBundleAnnotateCommand) annotateReleaseBundle(manager *lifecycle.LifecycleServicesManager,
	details services.ReleaseBundleDetails, queryParams services.CommonOptionalQueryParams, rbac *ReleaseBundleAnnotateCommand) error {
	return manager.AnnotateReleaseBundle(BuildAnnotationOperationParams(rbac, details, queryParams))
}

func BuildAnnotationOperationParams(rba *ReleaseBundleAnnotateCommand, details services.ReleaseBundleDetails,
	params services.CommonOptionalQueryParams) services.AnnotateOperationParams {
	return services.AnnotateOperationParams{
		RbTag: services.RbAnnotationTag{
			Tag:   rba.tag,
			Exist: rba.tagExist,
		},
		RbProps: services.RbAnnotationProps{
			Properties: buildProps(rba.props),
			Exist:      rba.propsExist,
		},
		RbDelProps: services.RbDelProps{
			Keys:  rba.deleteProps,
			Exist: rba.deletePropsExist,
		},
		RbDetails:   details,
		QueryParams: params,
		PropertyParams: services.CommonPropParams{
			Path: buildManifestPath(params.ProjectKey, details.ReleaseBundleName,
				details.ReleaseBundleVersion),
			Recursive: rba.recursive,
		},
		ArtifactoryUrl: services.ArtCommonParams{
			Url: rba.serverDetails.ArtifactoryUrl,
		},
	}
}

func buildProps(properties string) map[string][]string {
	if properties == "" {
		return make(map[string][]string)
	}
	props, err := utils.ParseProperties(properties)
	if err != nil {
		return make(map[string][]string)
	}
	return props.ToMap()
}
