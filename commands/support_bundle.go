package commands

import (
	"context"
	"errors"
	"fmt"
	"github.com/jfrog/jfrog-cli-core/artifactory/commands"
	"github.com/jfrog/jfrog-cli-core/plugins/components"
	"github.com/jfrog/jfrog-cli-core/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	serverIDFlag    = "server-id"
	targetServerID  = "target-server-id"
	downloadTimeout = "download-timeout"
	retryInterval   = "retry-interval"
	promptOptions   = "prompt-options"
	cleanup         = "cleanup"
	targetRepo      = "target-repo"
)

// GetSupportBundleCommand returns the description of the "support-bundle" command.
func GetSupportBundleCommand() components.Command {
	return components.Command{
		Name:        "support-bundle",
		Description: `Creates a Support Bundle and uploads it to JFrog Support "dropbox" service`,
		Aliases:     []string{"sb"},
		Arguments:   getArguments(),
		Flags:       getFlags(),
		EnvVars:     nil,
		Action:      supportBundleCmd,
	}
}

func getArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "case",
			Description: "JFrog Support case number.",
		},
	}
}

func getFlags() []components.Flag {
	return []components.Flag{
		components.StringFlag{
			Name: serverIDFlag,
			Description: "Artifactory server ID configured using the config command. " +
				"If not provided the default configuration will be used.",
		},
		components.StringFlag{
			Name: targetServerID,
			Description: "Artifactory server ID configured using the config command to be used as the target for " +
				"uploading the generated Support Bundle. If not provided JFrog support logs will be used.",
		},
		components.StringFlag{
			Name:         downloadTimeout,
			Description:  "The timeout for download.",
			DefaultValue: "10m",
		},
		components.StringFlag{
			Name:         retryInterval,
			Description:  "The duration to wait between retries.",
			DefaultValue: "5s",
		},
		components.BoolFlag{
			Name:        promptOptions,
			Description: "Ask for support bundle options or use Artifactory default options.",
		},
		components.BoolFlag{
			Name:         cleanup,
			Description:  "Delete the support bundle local temp file after upload.",
			DefaultValue: true,
		},
		components.StringFlag{
			Name:         targetRepo,
			Description:  "The target repository key where the support bundle will be uploaded to.",
			DefaultValue: "logs",
		},
	}
}

// SupportBundleCommandConfiguration defines the configuration of the "support-bundle" command.
type SupportBundleCommandConfiguration struct {
	CaseNumber          string
	JfrogSupportLogsURL string
}

type artifactoryServiceHelper struct{}

func (cw *artifactoryServiceHelper) GetConfig(serverID string, excludeRefreshableTokens bool) (*config.ArtifactoryDetails, error) {
	return commands.GetConfig(serverID, excludeRefreshableTokens)
}

func (cw *artifactoryServiceHelper) CreateInitialRefreshableTokensIfNeeded(artifactoryDetails *config.ArtifactoryDetails) error {
	return config.CreateInitialRefreshableTokensIfNeeded(artifactoryDetails)
}

func supportBundleCmd(componentContext *components.Context) error {
	ctx := context.Background()
	conf, err := parseArguments(componentContext)
	if err != nil {
		return err
	}

	artifactoryConfigHelper := &artifactoryServiceHelper{}
	rtDetails, err := getRtDetails(componentContext, artifactoryConfigHelper)
	if err != nil {
		return err
	}
	log.Debug(fmt.Sprintf("Using: %s...", rtDetails.Url))
	log.Output(fmt.Sprintf("Case number is %s", conf.CaseNumber))

	targetRtDetails, err := getTargetDetails(componentContext, artifactoryConfigHelper, conf)
	if err != nil {
		return err
	}

	client := &HTTPClient{RtDetails: rtDetails}
	targetClient := &HTTPClient{RtDetails: targetRtDetails}

	// 1. Create Support Bundle
	supportBundle, err := CreateSupportBundle(client, conf, getPromptOptions(componentContext))
	if err != nil {
		return err
	}

	// 2. Download Support Bundle
	supportBundleArchivePath, err := DownloadSupportBundle(ctx, client, getTimeout(componentContext),
		getRetryInterval(componentContext), supportBundle)
	if err != nil {
		return err
	}
	if shouldCleanup(componentContext) {
		defer deleteSupportBundleArchive(supportBundleArchivePath)
	}

	// 3. Upload Support Bundle
	return UploadSupportBundle(targetClient, conf, supportBundleArchivePath, getTargetRepo(componentContext), time.Now)
}

func deleteSupportBundleArchive(supportBundleArchivePath string) {
	log.Debug(fmt.Sprintf("Deleting generated support bundle: %s", supportBundleArchivePath))
	err := os.Remove(supportBundleArchivePath)
	if err != nil {
		log.Warn(fmt.Sprintf("Error occurred while deleting the generated support bundle archive: %+v", err))
	}
}

func parseArguments(ctx *components.Context) (*SupportBundleCommandConfiguration, error) {
	if len(ctx.Arguments) != 1 {
		return nil, errors.New("Wrong number of arguments. Expected: 1, " + "Received: " + strconv.Itoa(len(ctx.Arguments)))
	}
	var conf = new(SupportBundleCommandConfiguration)
	conf.CaseNumber = strings.TrimSpace(ctx.Arguments[0])
	// TODO change this when everything works correctly "https://supportlogs.jfrog.com/" (keep the trailing slash!)
	conf.JfrogSupportLogsURL = "https://supportlogs.jfrog.com.invalid/"
	return conf, nil
}
