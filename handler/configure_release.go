package handler

import (
	"fmt"
	"strings"

	common "github.com/mergermarket/cdflow2-config-common"
)

// ConfigureRelease runs before the release to provide and check config.
func (h *Handler) ConfigureRelease(request *common.ConfigureReleaseRequest, response *common.ConfigureReleaseResponse) error {
	team, err := h.getTeam(request.Config["team"])
	if err != nil {
		response.Success = false
		fmt.Fprintln(h.ErrorStream, err)
		return nil
	}

	response.AdditionalMetadata["team"] = team

	if !h.CheckInputConfiguration(request.Config, request.Env) {
		response.Success = false
		return nil
	}

	for buildID, reqs := range request.ReleaseRequirements {
		env := make(map[string]string)
		response.Env[buildID] = env

		credentials, err := h.awsSession.Config.Credentials.Get()
		if err != nil {
			fmt.Fprintf(h.ErrorStream, "Unable to fetch AWS credentials: %v.\n", err)
			response.Success = false
			return nil
		}

		region := *h.awsSession.Config.Region

		for _, need := range reqs.Needs {
			if need == "ecr" {
				env["AWS_ACCESS_KEY_ID"] = credentials.AccessKeyID
				env["AWS_SECRET_ACCESS_KEY"] = credentials.SecretAccessKey
				env["AWS_SESSION_TOKEN"] = credentials.SessionToken
				env["AWS_REGION"] = region
				env["AWS_DEFAULT_REGION"] = region

				for key, element := range request.Env {
					if strings.HasPrefix(key, "CDFLOW2_DOCKER_AUTH_") {
						env[key] = element
					}
				}

				repoURI, err := h.getECRRepository(request.Component)
				if err != nil {
					fmt.Fprintln(h.ErrorStream, err)
					response.Success = false
					return nil
				}

				if repoURI == "" {
					fmt.Fprintf(h.ErrorStream, "ECR repository '%s' does not exists, did you run 'setup' first?\n", request.Component)
					response.Success = false
					return nil
				}

				env["ECR_REPOSITORY"] = repoURI
				env["ECR_TAG"] = fmt.Sprintf("%s-%s", buildID, request.Version)
			} else {
				fmt.Fprintf(h.ErrorStream, "unable to satisfy %q need for %q build", need, buildID)
				response.Success = false
				return nil
			}
		}
	}

	if !h.CheckAWSResources() {
		response.Success = false
		return nil
	}

	return nil
}

// CheckAWSResources checks that the Release Bucket, Tf State Bucket & Tf Locks Table are present
func (handler *Handler) CheckAWSResources() bool {
	problems := 0
	fmt.Fprintf(handler.ErrorStream, "%s\n\n", handler.styles.au.Underline("Checking AWS resources..."))

	buckets, err := listBuckets(handler.getS3Client())
	if err != nil {
		fmt.Fprintf(handler.ErrorStream, "%v\n\n", err)
		return false
	}

	if ok, _ := handler.handleReleaseBucket(buckets); !ok {
		problems++
	}

	if ok, _ := handler.handleTfstateBucket(buckets); !ok {
		problems++
	}

	ok := handler.handleTflocksTable()
	if !ok {
		problems++
	}

	fmt.Fprintln(handler.ErrorStream, "")
	if problems > 0 {
		fmt.Fprintf(handler.ErrorStream, "To set up AWS resources, please run:\n\n  cdflow2 setup\n\n")
	}

	return problems == 0
}
